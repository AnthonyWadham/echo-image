package history

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/interact"
)

// TargetSync is one player's view of the recorded things this tick, each as its row: what screens draw
// and shots meet. Sent every tick and complete: a target missing from it is gone or hidden. One that
// died in the tick is in it once more, with when. The client's predictor places targets from it alone.
type TargetSync struct {
	Tick    uint64
	Targets cardinal.Slice[TargetState]
}

// Name is the event name.
func (TargetSync) Name() string { return "target_sync" }

// TargetState is one recorded thing's row this tick (past.Row), with its entity and serial.
type TargetState struct {
	EntityID uint32
	Serial   uint64 // its ref.Spawn serial: what tells it from a newer entity with its EntityID
	Kind     interact.Kind
	Owner    uint32 // its owner's Body: a Body owns itself
	Team     uint8
	X, Y     float64
	Radius   float64
	Tags     interact.Tags // what its effects make it from the next tick on, as that tick's rules read them
	Until    float64       // the part of the tick it lived: 1, or when it died in it
}
