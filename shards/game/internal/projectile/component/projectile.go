// Package component holds the projectile feature's ECS data — the leaf other features import to read
// a shot without depending on projectile's systems.
package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics2dcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// Projectile is a shot in flight. Source is the Body that launched it, credited with its damage.
type Projectile struct {
	Source    cardinal.EntityID
	Velocity  physics2dcomp.Vec2 // units per second
	Radius    float64
	Damage    float64
	Remaining float64 // distance left before it expires
}

// Name is the ECS component name.
func (Projectile) Name() string { return "projectile" }
