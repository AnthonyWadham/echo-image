package echo

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"

	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/team"
)

// EliminationSystem (Update, after damage resolves, before effects) puts the player whose Echo died
// out for the round (GDD §8): their Body gets the death effect (EchoTuning.DeathEffect, "out"), which
// the cast gate and the default shot rules read. Rounds will clear it. O(D log D) for D deaths.
type EliminationSystem struct {
	Tuning EchoTuning
}

func (s *EliminationSystem) Run(w *cardinal.World) {
	byTarget := func(a, b combatcomp.Died) int { return cmp.Compare(a.Target, b.Target) }
	for _, d := range slices.SortedFunc(w.SystemEvents[combatcomp.Died](), byTarget) {
		e := w.Entity(d.Target)
		if !e.Has[component.Echo]() {
			continue
		}
		w.EmitSystemEvent(effectcomp.EffectIntent{
			Target: uint32(e.Get[team.Owner]().Body), Kind: s.Tuning.DeathEffect, Ticks: status.Forever,
			Source: uint32(d.Target),
		})
	}
}
