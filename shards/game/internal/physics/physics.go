// Package physics wires the physics2d plugin into the world and owns the collision vocabulary
// (category bits, in categories.go) and the body-construction helpers that gameplay packages build on.
package physics

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// ServerTickRateHz is the fixed simulation rate shared by Cardinal ticks and the physics2d step.
const ServerTickRateHz = 20

// SecondsPerTick is dt — the wall-clock duration of one tick (1 / ServerTickRateHz). Scale per-second
// rates by this to stay tick-rate independent; never hardcode 0.05 (see CLAUDE.md → Tick-rate
// independence).
const SecondsPerTick = 1.0 / float64(ServerTickRateHz)

// Register wires the physics2d plugin (Box2D-backed 2D simulation) into the world. The plugin
// registers its own systems: an Init rebuild and a PreUpdate step that runs before game logic
// each tick, so contact/trigger events are ready for gameplay systems on Update.
//
// Gravity is zero for a top-down game (physics is used for collision on the ground plane, not
// falling). Set Gravity.Y negative for a side-view / platformer feel.
func Register(world *cardinal.World) {
	cardinal.RegisterPlugin(world, physics2d.NewPlugin(physics2d.Config{
		Gravity:  physics2d.Vec2{X: 0, Y: 0},
		TickRate: ServerTickRateHz,
	}))
}

// NewBody2D returns a PhysicsBody2D with Box2D-compatible defaults (Active, Awake and
// SleepingAllowed true; GravityScale 1) and the given collider shapes. Mirrors the plugin's
// component.NewPhysicsBody2D, which isn't re-exported at the physics2d package root. Set fields
// like Bullet or FixedRotation on the result before writing it to ECS.
func NewBody2D(bodyType physics2d.BodyType, shapes ...physics2d.ColliderShape) physics2d.PhysicsBody2D {
	return physics2d.PhysicsBody2D{
		BodyType:        bodyType,
		GravityScale:    1,
		Active:          true,
		Awake:           true,
		SleepingAllowed: true,
		Shapes:          shapes,
	}
}
