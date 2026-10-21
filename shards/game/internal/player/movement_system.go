package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/player/component"
)

// movableRow is a player Body we can drive.
type movableRow struct {
	Input    component.InputQueue
	Speed    component.MoveSpeed
	Velocity physics2d.Velocity2D
}

// MovementSystem (PreUpdate, after InputSystem and before the physics step, so a frame moves the
// Body the same tick it is acked) sets each Body's velocity from the frame applied this tick. A Body
// with no frame keeps its velocity until the input timeout, then stops. It is the only writer of a
// Body's Velocity2D. O(B).
type MovementSystem struct {
	Data *data.Plugin
}

func (s *MovementSystem) Run(w *cardinal.World) {
	timeout := data.Get[MovementTuning](s.Data).TimeoutTicks()
	tick := w.TickHeight()
	for body := range w.Contains[movableRow]().Iter() {
		q := body.Get[component.InputQueue]()
		f, ok := q.Applied(tick)
		if !ok {
			// No frame this tick: keep moving through a late frame, but stop a client that went silent.
			if tick > q.LastTick+timeout && body.Get[physics2d.Velocity2D]() != (physics2d.Velocity2D{}) {
				body.Set(physics2d.Velocity2D{})
			}
			continue
		}
		x, y := f.Move()
		speed := body.Get[component.MoveSpeed]().Value
		body.Set(physics2d.Velocity2D{Linear: physics2d.Vec2{X: x * speed, Y: y * speed}})
	}
}
