package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	abilitycomp "echo_image/shards/game/internal/ability/component"
)

// DamageIntent asks combat to damage a target. Damage sources (shot hits, areas such as anti-camp) send
// it to combat's queue with queue.Send; DamageResolveSystem drains it and is the only writer of Health,
// which is what makes the order intents arrive in irrelevant (see CLAUDE.md → Collision handling). It
// lives in this leaf so a source doesn't import combat's systems.
type DamageIntent struct {
	Target cardinal.EntityID
	Credit cardinal.EntityID // the Body credited with it
	Cause  abilitycomp.Cause // the cast that dealt it; zero for an area
	Amount float64
	Until  float64 // when in the tick it lands, 0 to 1: a hit's flight.End.Until, 1 for damage over the tick
}

// Name names its queue.
func (DamageIntent) Name() string { return "damage_intent" }
