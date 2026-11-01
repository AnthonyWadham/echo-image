package combat

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/combat/component"
)

// DeathSystem (Update, after the syncs) removes what died this tick. The syncs ran first, so each sent
// it once more, alive until it died. O(D log D) for D deaths.
type DeathSystem struct{}

func (s *DeathSystem) Run(w *cardinal.World) {
	byTarget := func(a, b component.Died) int { return cmp.Compare(a.Target, b.Target) }
	for _, d := range slices.SortedFunc(w.SystemEvents[component.Died](), byTarget) {
		// The physics plugin removes its Box2D body on the next PreUpdate reconcile.
		w.Entity(d.Target).Destroy()
		w.Logger().Debug().Uint32("target", uint32(d.Target)).Msg("entity died")
	}
}
