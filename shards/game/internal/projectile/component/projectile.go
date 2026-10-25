// Package component holds the projectile feature's ECS data — the leaf other features import to read
// a shot without depending on projectile's systems. It imports only the engine and ability's leaf (for
// Cause).
package component

import (
	physics2dcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"

	abilitycomp "echo_image/shards/game/internal/ability/component"
)

// Projectile is a shot in flight. Cause is the cast that launched it. The shot entity also carries the
// team.Owner its damage is credited to; a reflect would change the Owner and keep Cause.
type Projectile struct {
	Type      string // shot type ID, a key of projectile.ProjectileTuning.Shots: its rules
	Cause     abilitycomp.Cause
	Velocity  physics2dcomp.Vec2 // units per second
	Radius    float64
	Damage    float64
	Remaining float64 // distance left before it expires
}

// Name is the ECS component name.
func (Projectile) Name() string { return "projectile" }
