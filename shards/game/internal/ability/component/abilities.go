// Package component holds the ability feature's ECS data and its AbilityCast event — the leaf other
// features import to react to a cast without depending on ability's systems.
package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Slot is one ability in a kit. It can be cast from ReadyTick on.
type Slot struct {
	Ability   string // ability ID, a key of ability.AbilityTuning.Abilities
	ReadyTick uint64
}

// Abilities is a Body's kit. Input button N casts slot N.
type Abilities struct {
	Slots cardinal.Slice[Slot]
}

// Name is the ECS component name.
func (Abilities) Name() string { return "abilities" }
