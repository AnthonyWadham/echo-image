package echo

import "github.com/argus-labs/world-engine/pkg/cardinal"

// EchoSync is one player's view of the Echoes this tick. Sent every tick and complete: an Echo
// missing from it is dead or hidden.
type EchoSync struct {
	Tick   uint64
	Echoes cardinal.Slice[EchoState]
}

// Name is the event name.
func (EchoSync) Name() string { return "echo_sync" }

// EchoState is one Echo.
type EchoState struct {
	EntityID uint32
	PlayerID uint32
	Team     uint8
	X, Y     float64
	HP       float64
	MaxHP    float64
}
