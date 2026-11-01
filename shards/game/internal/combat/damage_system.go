package combat

import (
	"cmp"
	"maps"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"

	abilitycomp "echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/combat/component"
)

// Damageable is the search row for entities that carry Health. DamageResolveSystem uses it to look a
// damage target up by ID; any archetype containing Health matches, regardless of what else the entity
// has (physics body, AI, etc.). It's a query projection over the Health component, so it lives with
// the system that queries it, not in the component leaf.
type Damageable struct {
	Health component.Health
}

// DamageResolveSystem (Update, after every DamageIntent and HealIntent emitter) is the only writer of
// Health. For each target it takes the tick's damage and heals, and writes HP once: never above MaxHP,
// and at 0 or below the target dies: HP 0 and a component.Died, timed by the hit that took it there.
// Overkill counts, so a small heal can't save a target from a much bigger hit. Hits count in the order
// they land, ties in sorted order, since emit order can change after a restore. O(I log I) for I
// intents.
type DamageResolveSystem struct{}

func (s *DamageResolveSystem) Run(w *cardinal.World) {
	type change struct {
		hits []component.DamageIntent // in the order they land
		heal float64
	}
	changes := map[cardinal.EntityID]change{}
	for _, in := range slices.SortedFunc(w.SystemEvents[component.DamageIntent](), compareDamage) {
		c := changes[in.Target]
		c.hits = append(c.hits, in)
		changes[in.Target] = c

		// Debug, not Info: this is the combat hot path (one line per damage event per tick). The
		// client-facing signal for a hit belongs in an event sent with SendTo, not the server log.
		w.Logger().Debug().
			Uint32("target", uint32(in.Target)).
			Uint32("credit", uint32(in.Credit)).
			Float64("amount", in.Amount).
			Msg("damage applied")
	}
	for _, in := range slices.SortedFunc(w.SystemEvents[component.HealIntent](), compareHeal) {
		c := changes[in.Target]
		c.heal += in.Amount
		changes[in.Target] = c
	}
	targets := w.Contains[Damageable]()
	for _, id := range slices.Sorted(maps.Keys(changes)) {
		target, err := targets.GetByID(id)
		if err != nil {
			continue // target despawned this tick, or has no Health — nothing to apply
		}
		c, h := changes[id], target.Get[component.Health]()
		// It died when the first hit that left it at 0 or below landed, or at the tick's end with none.
		hp, timed, until := h.HP+c.heal, false, 1.0
		for _, hit := range c.hits {
			hp -= hit.Amount
			if hp <= 0 && !timed {
				timed, until = true, hit.Until
			}
		}
		if hp <= 0 {
			target.Set(component.Health{MaxHP: h.MaxHP})
			w.EmitSystemEvent(component.Died{Target: id, Until: until})
			continue
		}
		target.Set(component.Health{HP: min(hp, h.MaxHP), MaxHP: h.MaxHP})
	}
}

func compareDamage(a, b component.DamageIntent) int {
	return cmp.Or(cmp.Compare(a.Target, b.Target), cmp.Compare(a.Until, b.Until), cmp.Compare(a.Credit, b.Credit),
		compareCause(a.Cause, b.Cause), cmp.Compare(a.Amount, b.Amount))
}

func compareHeal(a, b component.HealIntent) int {
	return cmp.Or(cmp.Compare(a.Target, b.Target), cmp.Compare(a.Credit, b.Credit), compareCause(a.Cause, b.Cause),
		cmp.Compare(a.Amount, b.Amount))
}

func compareCause(a, b abilitycomp.Cause) int {
	return cmp.Or(cmp.Compare(a.Caster, b.Caster), cmp.Compare(a.Ability, b.Ability), cmp.Compare(a.Tick, b.Tick),
		cmp.Compare(a.Seq, b.Seq))
}
