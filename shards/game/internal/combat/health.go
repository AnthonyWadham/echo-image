// Package combat owns damageable state (Health) and the collision → damage pipeline:
// CollisionSystem classifies contacts into DamageIntents, and DamageResolveSystem applies them.
package combat

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Health is current/max hit points for any entity that can take damage.
type Health struct {
	HP    int
	MaxHP int
}

// Name is the ECS component name.
func (Health) Name() string { return "health" }

// Damageable is the search row for entities that carry Health. DamageResolveSystem uses it to
// look a damage target up by ID; any archetype containing Health matches, regardless of what
// else the entity has (physics body, AI, etc.).
type Damageable struct {
	Health cardinal.Ref[Health]
}
