package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// MoveFrame is one tick of a client's movement input: a direction and its sequence number.
type MoveFrame struct {
	Seq  uint32
	X, Y float64
}

// Input is a Body's movement input stream. Frames wait in Pending, in Seq order, and are applied one
// per tick. Ack is the Seq of the last applied frame and Tick the tick it was applied. Session tells a
// restarted client (Seq back at 1) from a stale frame.
type Input struct {
	Session uint32
	Pending cardinal.Slice[MoveFrame]
	Ack     uint32
	Tick    uint64
}

// Name is the ECS component name.
func (Input) Name() string { return "input" }
