package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Cause is the cast that started something. The shots, damage and effects a cast produces carry it, so
// what a cast started can be found later, even after credit moves (a reflect).
type Cause struct {
	Caster  cardinal.EntityID
	Ability string
	Tick    uint64
}

// AbilityCast is emitted the tick a Body casts an ability, as its Cause. Aim is a unit vector, or (0, 0)
// for an ability that isn't aimed.
type AbilityCast struct {
	Cause
	AimX, AimY float64
}

// Name is the system-event name.
func (AbilityCast) Name() string { return "ability_cast" }
