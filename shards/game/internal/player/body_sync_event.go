package player

import "github.com/argus-labs/world-engine/pkg/cardinal"

// BodySync is one player's view of the Bodies this tick, plus their own prediction data. Server time
// is Tick * TickSeconds; clients interpolate on it and predict their own Body in steps of TickSeconds.
// Sent every tick and complete, so a client that joins or reconnects needs nothing else.
type BodySync struct {
	Tick        uint64
	TickSeconds float64
	You         YouState
	Bodies      cardinal.Slice[BodyState]
}

// Name is the event name.
func (BodySync) Name() string { return "body_sync" }

// YouState is the receiving player's own data, for client-side prediction. InputAck counts only
// for the client run whose Input.Session matches InputSession.
type YouState struct {
	Body         uint32  // your Body's EntityID
	InputSession uint32  // Input.Session the ack belongs to
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
