package component

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// InputFrame is one tick of a client's input, with its sequence number. Move and Aim are directions
// of any length; read them through Move() and Aim(). Buttons has bit N set while the button for kit
// slot N is held (bit 0 is primary fire), so a new ability never changes this type.
type InputFrame struct {
	Seq          uint32
	MoveX, MoveY float64
	AimX, AimY   float64
	Buttons      uint32
}

// ButtonCount is how many buttons, and so kit slots, an InputFrame carries.
const ButtonCount = 32

// Pressed reports whether the button for kit slot is held.
func (f InputFrame) Pressed(slot int) bool {
	return slot >= 0 && slot < ButtonCount && f.Buttons&(1<<slot) != 0
}

// Move returns the movement direction as a unit vector, or (0, 0) for none or a bad value.
func (f InputFrame) Move() (x, y float64) { return unit(f.MoveX, f.MoveY) }

// Aim returns the aim direction as a unit vector, or (0, 0) for none or a bad value.
func (f InputFrame) Aim() (x, y float64) { return unit(f.AimX, f.AimY) }

// InputQueue is a Body's input stream. Frames wait in Pending, in Seq order, and one is applied per
// tick: Last is the frame applied most recently and LastTick the tick it was applied. Session tells a
// restarted client (Seq back at 1) from a stale frame.
type InputQueue struct {
	Session  uint32
	Pending  cardinal.Slice[InputFrame]
	Last     InputFrame
	LastTick uint64
}

func (InputQueue) Name() string { return "input_queue" }

// Ack is the Seq of the last applied frame.
func (q InputQueue) Ack() uint32 { return q.Last.Seq }

// Applied returns the frame applied at tick, if there was one. Systems act on input through it, so
// each frame acts once.
func (q InputQueue) Applied(tick uint64) (InputFrame, bool) {
	return q.Last, q.Last.Seq != 0 && q.LastTick == tick
}

// unit returns (x, y) scaled to length 1, or (0, 0) for a ~zero or non-finite input, so a bad client
// frame never reaches physics or gameplay as NaN.
func unit(x, y float64) (float64, float64) {
	mag := math.Hypot(x, y)
	if mag < 1e-9 || math.IsInf(mag, 0) || math.IsNaN(mag) {
		return 0, 0
	}
	return x / mag, y / mag
}
