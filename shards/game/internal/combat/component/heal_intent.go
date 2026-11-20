package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	abilitycomp "echo_image/shards/game/internal/ability/component"
)

// HealIntent asks DamageResolveSystem to heal Target, never above its MaxHP.
type HealIntent struct {
	Target cardinal.EntityID
	Credit cardinal.EntityID // the Body credited with it
	Cause  abilitycomp.Cause // the cast that healed, if any
	Amount float64
}

// Name names its queue.
func (HealIntent) Name() string { return "heal_intent" }
