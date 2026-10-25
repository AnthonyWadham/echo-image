package player

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/physics/mover"
	"echo_image/shards/game/internal/player/component"
)

// movableRow is a player Body we can drive. Body is read for its collider circle, Effects for what
// changes its move.
type movableRow struct {
	Input     component.InputQueue
	Speed     component.MoveSpeed
	Transform physics2d.Transform2D
	Velocity  physics2d.Velocity2D
	Body      physics2d.PhysicsBody2D
	Effects   effectcomp.Effects
}

// MovementSystem (PreUpdate, after InputSystem and the casts and before the physics plugin, so a frame
// moves the Body the same tick it is acked) moves each Body with Box2D's geometric mover: the map kinds
// in its mask stop it and it slides along them, while other Bodies don't block it. Effects feed it and
// never move a Body: a forced velocity replaces input, the folded speed scales input, and a pass drops
// map kinds from the mask. A Body whose center is inside a blocker first lands on the nearest clear
// spot inside the map, since the mover would walk it out the far side. It is the only writer of a Body's Transform2D
// and Velocity2D. O(B).
type MovementSystem struct {
	Data    *data.Plugin
	Physics *physics2d.Plugin
}

func (s *MovementSystem) Run(w *cardinal.World) {
	engine := s.Physics.Engine()
	timeout := data.Get[MovementTuning](s.Data).TimeoutTicks()
	tick := w.TickHeight()
	edge := box2d.QueryFilter{CategoryBits: uint64(interact.Body), MaskBits: uint64(interact.Bounds)}
	for body := range w.Contains[movableRow]().Iter() {
		st := body.Get[effectcomp.Effects]().Status(tick)
		filter := box2d.QueryFilter{CategoryBits: uint64(interact.Body), MaskBits: uint64(physics.MoveBlockers &^ st.Pass)}
		pose := body.Get[physics2d.Transform2D]()
		circle := box2d.Circle{Center: box2d.Vec2(pose.Position), Radius: radius(body)}
		if mover.Inside(engine, circle.Center, filter) {
			circle.Center, _ = mover.Land(engine, circle, filter, edge)
		}
		v := box2d.Vec2(velocity(body, st, tick, timeout))
		center, clipped := mover.Move(engine, circle, v, physics.SecondsPerTick, filter)
		pose.Position = physics2d.Vec2(center)
		body.Set(pose)
		body.Set(physics2d.Velocity2D{Linear: physics2d.Vec2(clipped)})
	}
}

// velocity is a Body's velocity this tick: forced by its effects, or else its input frame at its folded
// speed. With no frame this tick it keeps the last one, until the input timeout.
func velocity(body cardinal.Entity, st status.Status, tick, timeout uint64) physics2d.Vec2 {
	if st.Forced {
		return physics2d.Vec2{X: st.VelX, Y: st.VelY}
	}
	q := body.Get[component.InputQueue]()
	if _, ok := q.Applied(tick); !ok && tick > q.LastTick+timeout {
		return physics2d.Vec2{} // the client went silent
	}
	// The last frame, never Velocity2D: after a dash, that would coast at dash speed.
	x, y := q.Last.Move()
	speed := st.Speed(body.Get[component.MoveSpeed]().Value)
	return physics2d.Vec2{X: x * speed, Y: y * speed}
}
