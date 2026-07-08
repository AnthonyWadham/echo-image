package combat

import "github.com/argus-labs/world-engine/pkg/cardinal"

// DamageIntent is a deferred request to apply damage to a target entity. CollisionSystem emits these
// while classifying contacts; DamageResolveSystem drains them and applies the result. Nothing that
// emits an intent mutates game state — that happens only in DamageResolveSystem, which is what makes
// the order contacts arrive in irrelevant (see CLAUDE.md → Collision handling).
//
// It's a combat-internal system event today. If another feature ever needs to deal damage (an ability,
// an environmental hazard), promote it to a leaf the same way Health lives in combat/component, so the
// emitter doesn't drag in combat's systems.
type DamageIntent struct {
	Target cardinal.EntityID
	Source cardinal.EntityID
	Amount float64
}

// Name is the system-event name.
func (DamageIntent) Name() string { return "damage_intent" }
