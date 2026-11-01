package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/player/motion"
)

// InputFrame is one tick of a client's input, with its sequence number. Move and Aim are directions
// of any length; read them through Move() and Aim(). Buttons has bit N set while the button for kit
// slot N is held (bit 0 is primary fire), so a new ability never changes this type. ViewTick is the
// render tick on the client's screen when it made the frame: the client's shots meet Bodies and Echoes
// where it drew them then.
type InputFrame struct {
	Seq          uint32
	MoveX, MoveY float64
	AimX, AimY   float64
	Buttons      uint32
	ViewTick     float64
}

// ButtonCount is how many buttons, and so kit slots, an InputFrame carries.
const ButtonCount = 32

// Move returns the movement direction as a unit vector, or (0, 0) for none or a bad value.
func (f InputFrame) Move() (x, y float64) { return motion.Unit(f.MoveX, f.MoveY) }

// Aim returns the aim direction as a unit vector, or (0, 0) for none or a bad value.
func (f InputFrame) Aim() (x, y float64) { return motion.Unit(f.AimX, f.AimY) }

// InputQueue is a Body's input stream. Frames wait in Pending, in Seq order, and one is applied per
// tick: Last is the frame applied most recently and LastTick the tick it was applied. Session tells a
// restarted client (Seq back at 1) from a stale frame. ViewFloor is the largest ViewTick applied, from 0,
// kept across sessions: a frame's view below it, or not a number, is taken at it, so a client can lag
// but never pick an older moment for each shot. Target is how many frames its client should keep queued
// as of Coasted, the last tick the queue ran dry (TargetAt).
type InputQueue struct {
	Session   uint32
	Pending   cardinal.Slice[InputFrame]
	Last      InputFrame
	LastTick  uint64
	ViewFloor float64
	Target    uint32
	Coasted   uint64
}

func (InputQueue) Name() string { return "input_queue" }

// Ack is the Seq of the last applied frame.
func (q InputQueue) Ack() uint32 { return q.Last.Seq }

// Applied returns the frame applied at tick, if there was one. Systems act on input through it, so
// each frame acts once.
func (q InputQueue) Applied(tick uint64) (InputFrame, bool) {
	return q.Last, q.Last.Seq != 0 && q.LastTick == tick
}

// TargetAt is how many frames q's client should keep queued at tick: Target, less step for each calm
// ticks since the queue last ran dry, from one up to limit.
func (q InputQueue) TargetAt(tick uint64, step int, calm uint64, limit int) int {
	return min(max(int(q.Target)-step*int((tick-q.Coasted)/calm), 1), limit)
}

// Coast is q after its queue ran dry at tick: its target a step higher.
func (q InputQueue) Coast(tick uint64, step int, calm uint64, limit int) InputQueue {
	q.Target, q.Coasted = uint32(min(q.TargetAt(tick, step, calm, limit)+step, limit)), tick
	return q
}

// View is the render tick on the client's screen at tick: the last frame's ViewTick, moved on by the
// ticks since it was applied. False before any frame.
func (q InputQueue) View(tick uint64) (float64, bool) {
	return q.Last.ViewTick + float64(tick-q.LastTick), q.Last.Seq != 0
}
