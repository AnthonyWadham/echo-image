package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// DamageIntent is a deferred request to damage a target entity. Damage sources (projectile hits,
// combat's anti-camp) emit it; DamageResolveSystem drains it and applies the result. Nothing that
// emits an intent mutates Health — only DamageResolveSystem does, which is what makes the order
// intents arrive in irrelevant (see CLAUDE.md → Collision handling). It lives in this leaf so a
// source doesn't import combat's systems.
type DamageIntent struct {
	Target cardinal.EntityID
	Source cardinal.EntityID // the Body responsible
	Amount float64
}

// Name is the system-event name.
func (DamageIntent) Name() string { return "damage_intent" }
