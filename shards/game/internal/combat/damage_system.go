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
	Health component.Health
}

// DamageResolveSystem is the single place game state actually changes in response to combat. It
// runs on Update, registered after every intent emitter (projectile.FlightSystem, AntiCampSystem) so
// this tick's intents are visible. Applying intents here — rather than where damage is detected — is
// what makes hit ordering irrelevant: subtraction commutes, so any order of intents against the same
// target yields the same HP. If two intents kill a target, the first destroys it and the second's
// GetByID simply misses. This is also the natural home for future rules (armor, i-frames, loot).
type DamageResolveSystem struct{}

func (s *DamageResolveSystem) Run(w *cardinal.World) {
	targets := w.Contains[Damageable]()
	for intent := range w.SystemEvents[component.DamageIntent]() {
		target, err := targets.GetByID(intent.Target)
		if err != nil {
			continue // target despawned this tick, or has no Health — nothing to apply
		}

		h := target.Get[component.Health]()
		h.HP -= intent.Amount
		target.Set(h)

		// Debug, not Info: this is the combat hot path (one line per damage event per tick). The
		// client-facing signal for a hit belongs in an event sent with SendTo, not the server log.
		w.Logger().Debug().
			Uint32("target", uint32(intent.Target)).
			Uint32("source", uint32(intent.Source)).
			Float64("amount", intent.Amount).
			Float64("hp", h.HP).
			Msg("damage applied")

		if h.HP <= 0 {
			// Destroying the ECS entity is enough; the physics plugin removes its Box2D body on
			// the next PreUpdate reconcile when it sees the entity gone.
			target.Destroy()
			w.Logger().Debug().Uint32("target", uint32(intent.Target)).Msg("entity died")
		}
	}
}
