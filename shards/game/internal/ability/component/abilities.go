// Package component holds the ability feature's ECS data and its AbilityCast event — the leaf other
// features import to react to a cast without depending on ability's systems.
package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Cooldown is when an ability can be cast again.
type Cooldown struct {
	Ability   string // ability ID, a key of ability.AbilityTuning.Abilities
	ReadyTick uint64
}

// Abilities is a Body's cooldowns, one per ability ID it has cast, so a cooldown follows its ability
// into whichever slot holds it. The kit is the Body's champion's, in ability tuning.
type Abilities struct {
	Cooldowns cardinal.Slice[Cooldown]
}

// Name is the ECS component name.
func (Abilities) Name() string { return "abilities" }

// ReadyTick is when ability can be cast again: 0 if it never was cast.
func (a Abilities) ReadyTick(ability string) uint64 {
	if i := a.index(ability); i >= 0 {
		return a.Cooldowns.At(i).ReadyTick
	}
	return 0
}

// WithReadyTick returns a with ability castable again from tick. Set the component after it.
func (a Abilities) WithReadyTick(ability string, tick uint64) Abilities {
	c := Cooldown{Ability: ability, ReadyTick: tick}
	if i := a.index(ability); i >= 0 {
		a.Cooldowns = a.Cooldowns.With(i, c)
	} else {
		a.Cooldowns = a.Cooldowns.Append(c)
	}
	return a
}

func (a Abilities) index(ability string) int {
	return a.Cooldowns.IndexFunc(func(c Cooldown) bool { return c.Ability == ability })
}
