package projectile

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
)

// historyRow is a Body or Echo that keeps its rows.
type historyRow struct {
	History   component.TargetHistory
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D
}

// HistorySystem (Update, after effect.ResolveSystem, before the syncs) saves this tick's row of every Body
// and Echo, which is what this tick's syncs show of it. FlightSystem checks hit claims against the rows,
// at the view the shooter saw. It keeps rewind_seconds of them. O(T·R) for T Bodies and Echoes keeping R
// rows.
type HistorySystem struct {
	Data *data.Plugin
}

func (s *HistorySystem) Run(w *cardinal.World) {
	keep := int(data.Get[ProjectileTuning](s.Data).RewindTicks())
	tick := w.TickHeight()
	for e := range w.Contains[historyRow]().Iter() {
		if e.Get[physics2d.PhysicsBody2D]().Shapes.Len() == 0 {
			continue
		}
		rows := append(slices.Collect(e.Get[component.TargetHistory]().Rows.Values()), rowOf(e, tick))
		e.Set(component.TargetHistory{Rows: cardinal.SliceOf(rows[max(len(rows)-keep, 0):]...)})
	}
}

// rowOf is Body or Echo e as this tick's syncs show it: one circle at its origin, with the tags its
// effects give it from the next tick on. One that died this tick is removed before a claim reads it, so
// a row lives the whole tick.
func rowOf(e cardinal.Entity, tick uint64) flight.Row {
	return flight.Row{
		Tick:   tick,
		Center: box2d.Vec2(e.Get[physics2d.Transform2D]().Position),
		Radius: e.Get[physics2d.PhysicsBody2D]().Shapes.At(0).Radius,
		Tags:   tagsAt(e, tick+1),
		Until:  1,
	}
}

// tagsAt is the tags e's effects give it at tick.
func tagsAt(e cardinal.Entity, tick uint64) interact.Tags {
	if !e.Has[effectcomp.Effects]() {
		return 0
	}
	return e.Get[effectcomp.Effects]().Status(tick).Tags
}
