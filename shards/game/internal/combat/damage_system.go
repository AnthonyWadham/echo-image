package combat

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/combat/component"
)

// Damageable is the search row for entities that carry Health. DamageResolveSystem uses it to look a
// damage target up by ID; any archetype containing Health matches, regardless of what else the entity
// has (physics body, AI, etc.). It's a query projection over the Health component, so it lives with
// the system that queries it, not in the component leaf.
type Damageable struct {
	Health cardinal.Ref[component.Health]
}

// DamageResolveSystemState drains DamageIntents and owns the Health mutation.
type DamageResolveSystemState struct {
	cardinal.BaseSystemState

	Intents cardinal.WithSystemEventReceiver[DamageIntent]
	Targets cardinal.Contains[Damageable]
}

// DamageResolveSystem is the single place game state actually changes in response to combat. It
// runs on Update, registered after CollisionSystem so this tick's intents are visible. Applying
// intents here — rather than in the collision handlers — is what makes collision ordering
// irrelevant: subtraction commutes, so any order of intents against the same target yields the
// same HP. If two intents kill a target, the first destroys it and the second's GetByID simply
// misses. This is also the natural home for future rules (armor, i-frames, on-death loot).
func DamageResolveSystem(state *DamageResolveSystemState) {
	for intent := range state.Intents.Iter() {
		row, err := state.Targets.GetByID(intent.Target)
		if err != nil {
			continue // target despawned this tick, or has no Health — nothing to apply
		}

		h := row.Health.Get()
		h.HP -= intent.Amount
		row.Health.Set(h)

		// Debug, not Info: this is the combat hot path (one line per damage event per tick). The
		// client-facing signal for a hit belongs in a WithEvent emission, not the server log.
		state.Logger().Debug().
			Uint32("target", uint32(intent.Target)).
			Uint32("source", uint32(intent.Source)).
			Float64("amount", intent.Amount).
			Float64("hp", h.HP).
			Msg("damage applied")

		if h.HP <= 0 {
			// Destroying the ECS entity is enough; the physics plugin removes its Box2D body on
			// the next PreUpdate reconcile when it sees the entity gone.
			state.Targets.Destroy(intent.Target)
			state.Logger().Debug().Uint32("target", uint32(intent.Target)).Msg("entity died")
		}
	}
}
