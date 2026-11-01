package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/interact"
)

// EchoSync is one player's view of the Echoes this tick. Sent every tick and complete: an Echo
// missing from it is dead or hidden. One that died in the tick is in it once more.
type EchoSync struct {
	Tick   uint64
	Echoes cardinal.Slice[EchoState]
}

// Name is the event name.
func (EchoSync) Name() string { return "echo_sync" }

// EchoState is one Echo. Its position, radius, tags and Until are its projectilecomp.TargetHistory row.
type EchoState struct {
	EntityID uint32
	PlayerID uint32
	Owner    uint32 // its Body's EntityID
	Team     uint8
	X, Y     float64
	Radius   float64       // its collider circle: the hurtbox shots meet
	Tags     interact.Tags // what its effects make it from the next tick on, as that tick's rules read them
	HP       float64
	MaxHP    float64
	Until    float64 // the part of the tick it lived: 1, or when it died in it (sent once more, at HP 0)
}
