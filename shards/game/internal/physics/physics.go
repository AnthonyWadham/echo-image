// Package physics configures the physics2d plugin for the game and owns the collision vocabulary
// (category bits, in categories.go) and the body-construction helpers that gameplay packages build on.
package physics

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"

	"echo_image/shards/game/internal/physics/timestep"
)

// ServerTickRateHz is the fixed simulation rate shared by Cardinal ticks and the physics2d step. It is
// timestep.Rate, which the client predictor shares.
const ServerTickRateHz = timestep.Rate

// SecondsPerTick is dt — the wall-clock duration of one tick (1 / ServerTickRateHz). Scale per-second
// rates by this to stay tick-rate independent; never hardcode 0.05 (see CLAUDE.md → Tick-rate
// independence).
const SecondsPerTick = timestep.Seconds

// NewPlugin returns the physics2d plugin (Box2D-backed 2D simulation) for world.RegisterPlugin; keep
// it for queries (CircleSweep, Raycast, Engine). It is built apart from registering so a system
// registered before it can hold it. On RegisterPlugin it adds its own systems: an Init rebuild and a
// PreUpdate step that runs before game logic each tick, so contact/trigger events are ready for
// gameplay systems on Update.
//
// Gravity is zero for a top-down game (physics is used for collision on the ground plane, not
// falling). Set Gravity.Y negative for a side-view / platformer feel.
func NewPlugin() *physics2d.Plugin {
	return physics2d.NewPlugin(physics2d.Config{
		Gravity:  physics2d.Vec2{X: 0, Y: 0},
		TickRate: ServerTickRateHz,
	})
}

// NewBody2D returns a PhysicsBody2D with the plugin's Box2D defaults (Active, Awake and
// SleepingAllowed true; GravityScale 1) and the given collider shapes. Set fields like Bullet or
// FixedRotation on the result before writing it to ECS.
func NewBody2D(bodyType physics2d.BodyType, shapes ...physics2d.ColliderShape) physics2d.PhysicsBody2D {
	return component.NewPhysicsBody2D(bodyType, shapes...)
}

// EntityOf returns the entity a Box2D shape belongs to. physics2d keeps it in the body's user data.
func EntityOf(world *box2d.World, shape box2d.ShapeID) cardinal.EntityID {
	return cardinal.EntityID(uint32(world.BodyUserData(world.ShapeBody(shape))))
}

// WorldPoint returns a shape's local offset in world space, at pose.
func WorldPoint(pose physics2d.Transform2D, local physics2d.Vec2) physics2d.Vec2 {
	c, s := math.Cos(pose.Rotation), math.Sin(pose.Rotation)
	return physics2d.Vec2{X: pose.Position.X + c*local.X - s*local.Y, Y: pose.Position.Y + s*local.X + c*local.Y}
}
