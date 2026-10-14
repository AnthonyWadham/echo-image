package netsync

import "github.com/argus-labs/world-engine/pkg/cardinal"

// WorldState is one player's view of a tick. Server time is Tick * TickSeconds; clients
// interpolate on it and predict their own Body in steps of TickSeconds. Each state is complete, so a
// client that joins or reconnects needs nothing else.
type WorldState struct {
	Tick        uint64
	TickSeconds float64
	You         YouState
	Bodies      cardinal.Slice[BodyState]
	Echoes      cardinal.Slice[EchoState]
}

// Name is the event name.
func (WorldState) Name() string { return "world_state" }

// YouState is the receiving player's own data, for client-side prediction. InputAck counts only
// for the client run whose Move.Session matches InputSession.
type YouState struct {
	Body         uint32  // your Body's EntityID
	InputSession uint32  // Move.Session the ack belongs to
	InputAck     uint32  // Seq of the last applied frame, included in your Body's position
	MoveSpeed    float64 // units per second at full input
}

// BodyState is one player Body.
type BodyState struct {
	EntityID uint32
	PlayerID uint32
	Nickname string
	Team     uint8
	X, Y     float64
}

// EchoState is one Echo.
type EchoState struct {
	EntityID uint32
	PlayerID uint32
	Team     uint8
	X, Y     float64
	HP       float64
	MaxHP    float64
}
