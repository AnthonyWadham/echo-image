package player

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/pkg/levelstate"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/player/motion"
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
// moves the Body the same tick it is acked) moves each Body one tick with motion.Step, the code the
// client predictor runs too: Box2D's geometric mover through the live map's world, where the map kinds
// in its mask stop it and it slides along them, while other Bodies don't block it. Effects feed it and
// never move a Body. It is the only writer of a Body's Transform2D and Velocity2D. O(B).
type MovementSystem struct {
	Tuning MovementTuning
	Levels *levelstate.Plugin
}

func (s *MovementSystem) Run(w *cardinal.World) {
	engine := s.Levels.Engine()
	timeout := s.Tuning.TimeoutTicks()
	tick := w.TickHeight()
	for body := range w.Contains[movableRow]().Iter() {
		pose := body.Get[physics2d.Transform2D]()
		center, v := motion.Step(engine, motion.Body{
			Center: box2d.Vec2(pose.Position),
			Radius: radius(body),
			Move:   move(body, tick, timeout),
			Speed:  body.Get[component.MoveSpeed]().Value,
		}, body.Get[effectcomp.Effects]().Status(tick), physics.SecondsPerTick)
		pose.Position = physics2d.Vec2(center)
		body.Set(pose)
		body.Set(physics2d.Velocity2D{Linear: physics2d.Vec2(v)})
	}
}

// move is a Body's input direction this tick. With no frame this tick it keeps the last one, until the
// input timeout.
func move(body cardinal.Entity, tick, timeout uint64) box2d.Vec2 {
	q := body.Get[component.InputQueue]()
	if _, ok := q.Applied(tick); !ok && tick > q.LastTick+timeout {
		return box2d.Vec2{} // the client went silent
	}
	// The last frame, never Velocity2D: after a dash, that would coast at dash speed.
	x, y := q.Last.Move()
	return box2d.Vec2{X: x, Y: y}
}
