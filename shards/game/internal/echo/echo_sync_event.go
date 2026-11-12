package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/effect/status"
)

// EchoSync is one player's view of the Echoes this tick. Sent every tick and complete: an Echo
// missing from it is dead or hidden. One that died in the tick is in it once more.
type EchoSync struct {
	Tick   uint64
	Echoes cardinal.Slice[EchoState]
}

// Name is the event name.
func (EchoSync) Name() string { return "echo_sync" }

// EchoState is what an Echo's row in history.TargetSync lacks: whose it is and its health. Its row has
// its owner, team, spot, size, tags and when it died in the tick.
type EchoState struct {
	EntityID uint32
	Serial   uint64 // its ref.Spawn serial, as its row's
	PlayerID uint32
	Effects  cardinal.Slice[status.Shown] // its effect kinds from the next tick on, for their visuals
	HP       float64                      // 0 the tick it dies, when it's sent once more
	MaxHP    float64
}
