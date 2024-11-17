package main

import (
	"strconv"

	"github.com/firefly-zero/firefly-go/firefly"
	"github.com/orsinium-labs/tinymath"
)

// General constants for gameplay mechanics.
const (
	period        = 10              // Frames per movement cycle.
	snakeWidth    = 7               // Width of the snake's body.
	segmentLen    = 14              // Length of each segment of the snake.
	maxDirDiff    = 0.1             // Maximum angular change per input.
	HungerPeriod  = 6 * 60          // Time (in frames) until the snake loses points from hunger.
	IFrames       = 60              // Frames of invulnerability after a collision.
	appleRadius   = 5               // Radius of the apple.
	appleDiameter = appleRadius * 2 // Diameter of the apple.
)

// Game states for snake behavior.
type State uint8

const (
	Moving  State = 0 // Snake is moving normally.
	Eating  State = 1 // Snake has eaten an apple and will grow.
	Growing State = 2 // Snake is growing, tail is stationary temporarily.
)

// Global variables for game entities.
var (
	frame  int          // Current game frame.
	font   firefly.Font // Font used for rendering text.
	apple  Apple        // The game apple.
	snakes []*Snake     // List of all snakes in the game.
	score  Score        // Player's score.
)

// Segment represents one section of the snake's body.
type Segment struct {
	Head firefly.Point // Start point of this segment.
	Tail *Segment      // Pointer to the next segment.
}

// Render draws a segment of the snake.
func (s *Segment) Render(frame int, state State) {
	if s.Tail == nil {
		return // Tail doesn't exist, nothing to render.
	}
	start := s.Head
	end := s.Tail.Head
	start.X, end.X = denormalizeX(start.X, end.X)
	start.Y, end.Y = denormalizeY(start.Y, end.Y)
	// Shorten the tail segment during growth animation.
	if s.Tail.Tail == nil && state != Growing {
		end.X = start.X + (end.X-start.X)*(period-frame)/period
		end.Y = start.Y + (end.Y-start.Y)*(period-frame)/period
	}
	drawSegment(start, end)
}

// Snake represents a single snake controlled by a player.
type Snake struct {
	Peer         firefly.Peer  // Player ID controlling the snake.
	Head         *Segment      // The head segment of the snake.
	Mouth        firefly.Point // Current position of the snake's mouth.
	Eye          firefly.Point // Position the snake is "looking" at.
	BlinkCounter int           // Counter for blinking animation.
	BlinkMaxTime int           // Maximum duration for a blink.
	Dir          float32       // Movement direction in radians.
	state        State         // Current state of the snake (Moving, Eating, Growing).
}

// Create a new snake for a given peer.
func NewSnake(peer firefly.Peer) *Snake {
	shift := 10 + snakeWidth + int(peer)*20
	return &Snake{
		Peer: peer,
		Head: &Segment{
			Head: firefly.Point{X: segmentLen * 2, Y: shift},
			Tail: &Segment{
				Head: firefly.Point{X: segmentLen, Y: shift},
				Tail: nil,
			},
		},
	}
}

// Update handles all snake logic for each frame.
func (s *Snake) Update(frame int, apple *Apple) {
	frame = frame % period
	pad, pressed := firefly.ReadPad(s.Peer)
	if pressed {
		s.setDir(pad) // Update direction based on input.
	}
	if frame == 0 {
		s.shift() // Move the snake's segments forward.
	}
	s.updateMouth(frame)
	s.updateEye(apple.Pos)
}

// Adjust the snake's direction based on player input.
func (s *Snake) setDir(pad firefly.Pad) {
	dirDiff := pad.Azimuth().Radians() - s.Dir
	if tinymath.IsNaN(dirDiff) {
		return
	}
	// Ensure the snake takes the shortest turn direction.
	if dirDiff > tinymath.Pi {
		dirDiff = -maxDirDiff
	} else if dirDiff < -tinymath.Pi {
		dirDiff = maxDirDiff
	}
	// Smooth the turn within the allowed angular change.
	if dirDiff > maxDirDiff {
		s.Dir += maxDirDiff
	} else if dirDiff < -maxDirDiff {
		s.Dir -= maxDirDiff
	} else {
		s.Dir += dirDiff
	}
	// Keep the direction within the 0-360° range.
	if s.Dir < 0 {
		s.Dir += tinymath.Tau
	}
	if s.Dir > tinymath.Tau {
		s.Dir -= tinymath.Tau
	}
}

// Update the snake's "eye" to look at the apple.
func (s *Snake) updateEye(apple firefly.Point) {
	lookX := float32(apple.X - s.Mouth.X)
	lookY := float32(apple.Y - s.Mouth.Y)
	lookLen := tinymath.Hypot(lookX, lookY)
	dX := lookX * 3 / lookLen
	dY := lookY * 3 / lookLen
	s.Eye = firefly.Point{
		X: s.Mouth.X + int(dX),
		Y: s.Mouth.Y + int(dY),
	}
	s.BlinkCounter += int(firefly.GetRandom() % 5)
	if s.BlinkCounter > s.BlinkMaxTime {
		s.BlinkCounter = 0
		s.BlinkMaxTime = int(100 + firefly.GetRandom()%100)
	}
}

// Move the snake's segments forward.
func (s *Snake) shift() {
	shiftX := tinymath.Cos(s.Dir) * segmentLen
	shiftY := tinymath.Sin(s.Dir) * segmentLen
	head := firefly.Point{
		X: normalizeX(s.Head.Head.X + int(shiftX)),
		Y: normalizeY(s.Head.Head.Y - int(shiftY)),
	}
	if s.state == Growing {
		s.Head = &Segment{
			Head: head,
			Tail: s.Head,
		}
		s.state = Moving
		return
	}
	if s.state == Eating {
		s.state = Growing
	}
	segment := s.Head
	for segment != nil {
		oldHead := segment.Head
		segment.Head = head
		head = oldHead
		segment = segment.Tail
	}
}

// Update the position of the snake's mouth.
func (s *Snake) updateMouth(frame int) {
	neck := s.Head.Head
	headLen := float32(segmentLen) * float32(frame) / float32(period)
	shiftX := tinymath.Cos(s.Dir) * headLen
	shiftY := tinymath.Sin(s.Dir) * headLen
	s.Mouth = firefly.Point{
		X: normalizeX(neck.X + int(shiftX)),
		Y: normalizeY(neck.Y - int(shiftY)),
	}
}

// Check if the snake eats the apple and grow if it does.
func (s *Snake) TryEat(apple *Apple, score *Score) {
	x := apple.Pos.X - s.Mouth.X
	y := apple.Pos.Y - s.Mouth.Y
	distance := tinymath.Hypot(float32(x), float32(y))
	if distance > appleRadius+snakeWidth/2 {
		return
	}
	s.state = Eating
	apple.Move()
	score.Inc()
	for s.Collides(apple.Pos) {
		apple.Move()
	}
}

// Check if a point is within the snake's body.
func (s Snake) Collides(p firefly.Point) bool {
	segment := s.Head.Tail
	for segment != nil {
		if segment.Tail != nil {
			ph := segment.Head
			pt := segment.Tail.Head
			ph.X, pt.X = denormalizeX(ph.X, pt.X)
			ph.Y, pt.Y = denormalizeY(ph.Y, pt.Y)
			bbox := NewBBox(ph, pt, snakeWidth/2)
			if bbox.Contains(p) {
				return true
			}
		}
		segment = segment.Tail
	}
	return false
}

// Render the entire snake, including its segments and head.
func (s Snake) Render(frame int) {
	frame = frame % period
	segment := s.Head
	for segment != nil {
		segment.Render(frame, s.state)
		segment = segment.Tail
	}
	s.renderHead()
}

// Render the head of the snake.
func (s Snake) renderHead() {
	neck := s.Head.Head
	mouth := s.Mouth
	// Denormalize positions to handle wrapping around screen edges.
	neck.X, mouth.X = denormalizeX(neck.X, mouth.X)
	neck.Y, mouth.Y = denormalizeY(neck.Y, mouth.Y)
	// Draw the segment between the neck and the mouth.
	drawSegment(neck, mouth)

	// Render styles for the head, showing collision status.
	style := firefly.Style{FillColor: firefly.ColorWhite}
	if s.Collides(mouth) {
		style.FillColor = firefly.ColorRed // Change to red if colliding.
	}

	// Render the head as a series of concentric circles for effect.
	firefly.DrawCircle(
		firefly.Point{
			X: mouth.X - snakeWidth/2 - 1,
			Y: mouth.Y - snakeWidth/2 - 1,
		},
		snakeWidth+2, firefly.Style{FillColor: firefly.ColorBlue},
	)
	firefly.DrawCircle(
		firefly.Point{
			X: mouth.X - snakeWidth/2,
			Y: mouth.Y - snakeWidth/2,
		},
		snakeWidth, firefly.Style{FillColor: firefly.ColorLightBlue},
	)
	firefly.DrawCircle(
		firefly.Point{
			X: s.Mouth.X - snakeWidth/2 + 1,
			Y: s.Mouth.Y - snakeWidth/2 + 1,
		},
		snakeWidth-2, style,
	)

	// Render the snake's "eye".
	s.renderEye()
}

// Render the snake's eye.
func (s Snake) renderEye() {
	// Draw a small black circle for the eye.
	firefly.DrawCircle(
		firefly.Point{
			X: s.Eye.X - snakeWidth/8,
			Y: s.Eye.Y - snakeWidth/8,
		},
		snakeWidth/4, firefly.Style{FillColor: firefly.ColorBlack},
	)

	// Blinking animation: render a blue overlay when blinking.
	if s.BlinkCounter < 20 {
		firefly.DrawCircle(
			firefly.Point{
				X: s.Mouth.X - snakeWidth/2 + 1,
				Y: s.Mouth.Y - snakeWidth/2 + 1,
			},
			snakeWidth-2, firefly.Style{FillColor: firefly.ColorLightBlue},
		)
	}
}

// Render the segment and ghost segments if the snake wraps around screen edges.
func drawSegment(start, end firefly.Point) {
	// Render the main segment.
	drawSegmentExactlyAt(start, end)
	// Draw "ghost" segments for edge wrapping.
	drawSegmentExactlyAt(
		firefly.Point{X: start.X - firefly.Width, Y: start.Y},
		firefly.Point{X: end.X - firefly.Width, Y: end.Y},
	)
	drawSegmentExactlyAt(
		firefly.Point{X: start.X, Y: start.Y - firefly.Height},
		firefly.Point{X: end.X, Y: end.Y - firefly.Height},
	)
	drawSegmentExactlyAt(
		firefly.Point{X: start.X - firefly.Width, Y: start.Y - firefly.Height},
		firefly.Point{X: end.X - firefly.Width, Y: end.Y - firefly.Height},
	)
}

// Render a segment at a specific position.
func drawSegmentExactlyAt(start, end firefly.Point) {
	// Draw a line segment for the snake's body.
	firefly.DrawLine(
		start, end,
		firefly.LineStyle{
			Color: firefly.ColorBlue,
			Width: snakeWidth,
		},
	)
	// Draw a circular cap at the segment's endpoint.
	firefly.DrawCircle(
		firefly.Point{
			X: end.X - snakeWidth/2,
			Y: end.Y - snakeWidth/2,
		},
		snakeWidth,
		firefly.Style{
			FillColor: firefly.ColorBlue,
		},
	)
}

// Helper: Normalize the X coordinate to stay within screen bounds.
func normalizeX(x int) int {
	if x >= firefly.Width {
		x -= firefly.Width
	} else if x < 0 {
		x += firefly.Width
	}
	return x
}

// Helper: Normalize the Y coordinate to stay within screen bounds.
func normalizeY(y int) int {
	if y >= firefly.Height {
		y -= firefly.Height
	} else if y < 0 {
		y += firefly.Height
	}
	return y
}

// Helper: Denormalize X coordinates for rendering across edges.
func denormalizeX(start, end int) (int, int) {
	if start-end > 30 {
		end += firefly.Width
	} else if end-start > 30 {
		start += firefly.Width
	}
	return start, end
}

// Helper: Denormalize Y coordinates for rendering across edges.
func denormalizeY(start, end int) (int, int) {
	if start-end > 30 {
		end += firefly.Height
	} else if end-start > 30 {
		start += firefly.Height
	}
	return start, end
}

// Bounding box for collision detection.
type BBox struct {
	left  firefly.Point
	right firefly.Point
}

// Create a new bounding box with margins.
func NewBBox(start, end firefly.Point, margin int) BBox {
	left := start.ComponentMin(end)
	right := start.ComponentMax(end)
	left.X -= margin
	right.X += margin
	left.Y -= margin
	right.Y += margin
	return BBox{left: left, right: right}
}

// Check if a point lies within the bounding box.
func (b BBox) Contains(p firefly.Point) bool {
	return !(p.X < b.left.X || p.X > b.right.X || p.Y < b.left.Y || p.Y > b.right.Y)
}

// Apple represents the collectible item in the game.
type Apple struct {
	Pos firefly.Point // Current position of the apple.
}

// Create a new apple at a random position.
func NewApple() Apple {
	a := Apple{}
	a.Move()
	return a
}

// Move the apple to a new random position.
func (a *Apple) Move() {
	a.Pos = firefly.Point{
		X: int(firefly.GetRandom()%(firefly.Width-appleRadius*2)) + appleRadius,
		Y: int(firefly.GetRandom()%(firefly.Height-appleRadius*2)) + appleRadius,
	}
}

// Render the apple on the screen.
func (a *Apple) Render() {
	firefly.DrawCircle(
		firefly.Point{X: a.Pos.X - appleRadius, Y: a.Pos.Y - appleRadius},
		appleDiameter,
		firefly.Style{FillColor: firefly.ColorRed},
	)
	firefly.DrawLine(
		a.Pos,
		firefly.Point{X: a.Pos.X + appleRadius, Y: a.Pos.Y - appleRadius},
		firefly.LineStyle{Color: firefly.ColorGreen, Width: 3},
	)
}

// Score represents the player's score and hunger system.
type Score struct {
	val     int // Current score.
	iframes int // Frames of invincibility.
	hunger  int // Frames remaining until hunger penalty.
}

// Create a new Score instance.
func NewScore() Score {
	return Score{
		hunger:  HungerPeriod,
		iframes: IFrames,
	}
}

// Update the score and handle hunger or collisions.
func (s *Score) Update(snake *Snake) {
	if s.iframes > 0 {
		s.iframes--
	}
	if s.hunger == 0 {
		s.Dec() // Penalize for hunger.
		s.hunger = HungerPeriod
	} else {
		s.hunger--
	}
	if snake.Collides(snake.Mouth) {
		s.Dec()
	}
}

// Increment the score when the snake eats an apple.
func (s *Score) Inc() {
	s.hunger = HungerPeriod
	s.val++
}

// Decrement the score on collision or hunger penalty.
func (s *Score) Dec() {
	if s.iframes > 0 {
		return
	}
	s.iframes = IFrames
	if s.val > 0 {
		s.val -= (s.val / 5) + 1
	}
}

// Render the score on the screen.
func (s Score) Render() {
	firefly.DrawText(
		strconv.Itoa(s.val), font,
		firefly.Point{X: 10, Y: 10},
		firefly.ColorDarkBlue,
	)
}

// Main game functions: boot, update, and render.
func boot() {
	font = firefly.LoadROMFile("font").Font()
	apple = NewApple()
	peers := firefly.GetPeers()
	snakes = make([]*Snake, peers.Len())
	for i, peer := range peers.Slice() {
		snakes[i] = NewSnake(peer)
	}
	score = NewScore()
}

func update() {
	frame++
	for _, snake := range snakes {
		snake.Update(frame, &apple)
		snake.TryEat(&apple, &score)
		score.Update(snake)
	}
}

func render() {
	firefly.ClearScreen(firefly.ColorWhite)
	apple.Render()
	for _, snake := range snakes {
		snake.Render(frame)
	}
	score.Render()
}

// Entry point for the Firefly game.
func init() {
	firefly.Boot = boot
	firefly.Update = update
	firefly.Render = render
	firefly.Cheat = cheat
}

// Cheat codes for debugging.
func cheat(c, v int) int {
	switch c {
	case 1: // Move apple to a new position.
		apple.Move()
		return 1
	case 2: // Increase score.
		for i := 0; i < int(v); i++ {
			score.Inc()
		}
		return score.val
	case 3: // Decrease score.
		for i := 0; i < int(v); i++ {
			score.Dec()
		}
		return score.val
	default:
		return 0
	}
}

func main() {

}
