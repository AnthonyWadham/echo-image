package player

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/physics/mover"
	"echo_image/shards/game/internal/player/component"
)

// movableRow is a player Body we can drive. Body is read for its collider circle.
type movableRow struct {
	Input     component.InputQueue
	Speed     component.MoveSpeed
	Transform physics2d.Transform2D
	Velocity  physics2d.Velocity2D
	Body      physics2d.PhysicsBody2D
}

// MovementSystem (PreUpdate, after InputSystem and before the physics plugin, so a frame moves the
// Body the same tick it is acked) moves each Body by its velocity with Box2D's geometric mover: walls
// stop it and it slides along them, while other Bodies don't block it. The velocity comes from the
// frame applied this tick; a Body with no frame keeps its velocity until the input timeout, then
// stops. It is the only writer of a Body's Transform2D and Velocity2D. O(B).
type MovementSystem struct {
	Data    *data.Plugin
	Physics *physics2d.Plugin
}

func (s *MovementSystem) Run(w *cardinal.World) {
	engine := s.Physics.Engine()
	timeout := data.Get[MovementTuning](s.Data).TimeoutTicks()
	tick := w.TickHeight()
	filter := box2d.QueryFilter{CategoryBits: physics.CategoryBody, MaskBits: physics.CategoryEnvironment}
	for body := range w.Contains[movableRow]().Iter() {
		pose := body.Get[physics2d.Transform2D]()
		circle := box2d.Circle{
			Center: box2d.Vec2(pose.Position),
			Radius: body.Get[physics2d.PhysicsBody2D]().Shapes.At(0).Radius, // spawnBody's one circle
		}
		v := box2d.Vec2(velocity(body, tick, timeout))
		center, clipped := mover.Move(engine, circle, v, physics.SecondsPerTick, filter)
		pose.Position = physics2d.Vec2(center)
		body.Set(pose)
		// Clipped, so a Body coasting through a late frame doesn't keep pushing into a wall.
		body.Set(physics2d.Velocity2D{Linear: physics2d.Vec2(clipped)})
	}
}

// velocity is a Body's velocity this tick: from the frame applied this tick, or else its last one.
func velocity(body cardinal.Entity, tick, timeout uint64) physics2d.Vec2 {
	q := body.Get[component.InputQueue]()
	if f, ok := q.Applied(tick); ok {
		x, y := f.Move()
		speed := body.Get[component.MoveSpeed]().Value
		return physics2d.Vec2{X: x * speed, Y: y * speed}
	}
	// No frame this tick: keep moving through a late frame, but stop a client that went silent.
	if tick > q.LastTick+timeout {
		return physics2d.Vec2{}
	}
	return body.Get[physics2d.Velocity2D]().Linear
}
