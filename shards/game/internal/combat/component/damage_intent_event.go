package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	abilitycomp "echo_image/shards/game/internal/ability/component"
)

// DamageIntent is a deferred request to damage a target entity. Damage sources (projectile hits,
// areas such as anti-camp) emit it; DamageResolveSystem drains it and applies the result. Nothing that
// emits an intent mutates Health — only DamageResolveSystem does, which is what makes the order
// intents arrive in irrelevant (see CLAUDE.md → Collision handling). It lives in this leaf so a
// source doesn't import combat's systems.
type DamageIntent struct {
	Target cardinal.EntityID
	Credit cardinal.EntityID // the Body credited with it
	Cause  abilitycomp.Cause // the cast that dealt it; zero for an area
	Amount float64
	Until  float64 // when in the tick it lands, 0 to 1: a hit's flight.End.Until, 1 for damage over the tick
}

// Name is the system-event name.
func (DamageIntent) Name() string { return "damage_intent" }
