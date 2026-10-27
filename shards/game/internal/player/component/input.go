package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/player/motion"
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

// Move returns the movement direction as a unit vector, or (0, 0) for none or a bad value.
func (f InputFrame) Move() (x, y float64) { return motion.Unit(f.MoveX, f.MoveY) }

// Aim returns the aim direction as a unit vector, or (0, 0) for none or a bad value.
func (f InputFrame) Aim() (x, y float64) { return motion.Unit(f.AimX, f.AimY) }

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
