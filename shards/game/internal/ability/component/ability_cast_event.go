package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// AbilityCast is emitted the tick a Body casts an ability. Aim is a unit vector, or (0, 0) for an
// ability that isn't aimed.
type AbilityCast struct {
	Caster     cardinal.EntityID
	Ability    string
	AimX, AimY float64
}

// Name is the system-event name.
func (AbilityCast) Name() string { return "ability_cast" }
