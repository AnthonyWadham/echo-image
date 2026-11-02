// Package component holds the ability feature's ECS data and its AbilityCast event — the leaf other
// features import to react to a cast without depending on ability's systems.
package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/ability/cast"
)

// Abilities is a Body's cooldowns, one per ability ID it has cast, so a cooldown follows its ability
// into whichever slot holds it. The kit is the Body's champion's, in ability tuning. ability.EquipSystem
// gives a claimed Body an empty one; after that only ability.CastSystem writes it, with cast.Gate's
// cooldowns.
type Abilities struct {
	Cooldowns cardinal.Slice[cast.Cooldown]
}

// Name is the ECS component name.
func (Abilities) Name() string { return "abilities" }

// ReadyTick is when ability can be cast again: 0 if it never was cast.
func (a Abilities) ReadyTick(ability string) uint64 {
	if i := a.Cooldowns.IndexFunc(func(c cast.Cooldown) bool { return c.Ability == ability }); i >= 0 {
		return a.Cooldowns.At(i).ReadyTick
	}
	return 0
}
