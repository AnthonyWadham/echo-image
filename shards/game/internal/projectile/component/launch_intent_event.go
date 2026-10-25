package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics2dcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"

	abilitycomp "echo_image/shards/game/internal/ability/component"
)

// LaunchIntent asks projectile.LaunchSystem to launch a shot of type Shot this tick, from From along
// Dir (a unit vector), for Owner on Team. It lives in this leaf so a sender doesn't import projectile's
// systems.
type LaunchIntent struct {
	Shot  string // a key of projectile.ProjectileTuning.Shots
	Cause abilitycomp.Cause
	Owner cardinal.EntityID
	Team  uint8
	From  physics2dcomp.Vec2
	Dir   physics2dcomp.Vec2
}

// Name is the system-event name.
func (LaunchIntent) Name() string { return "launch_intent" }
