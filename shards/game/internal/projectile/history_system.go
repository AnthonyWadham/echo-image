package projectile

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/internal/team"
)

// historyRow is a target that keeps its rows.
type historyRow struct {
	History   component.TargetHistory
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D
	Team      team.Team
	Owner     team.Owner
}

// HistorySystem (Update, after effect.ResolveSystem, before the syncs) saves this tick's row of every
// target, which is what this tick's syncs show of it. FlightSystem flies predicted shots against the
// rows, at the view their client drew, and TargetSyncSystem sends them to the predictors. It keeps those
// of every view rewind_seconds back (KeptRows). O(T·R) for T targets keeping R rows.
type HistorySystem struct {
	Data *data.Plugin
}

func (s *HistorySystem) Run(w *cardinal.World) {
	keep := int(data.Get[ProjectileTuning](s.Data).KeptRows())
	tick := w.TickHeight()
	died := map[cardinal.EntityID]float64{}
	for d := range w.SystemEvents[combatcomp.Died]() {
		died[d.Target] = d.Until
	}
	for e := range w.Contains[historyRow]().Iter() {
		if e.Get[physics2d.PhysicsBody2D]().Shapes.Len() == 0 {
			continue
		}
		row := rowOf(e, tick)
		if until, ok := died[e.ID()]; ok {
			row.Until = until
		}
		rows := append(slices.Collect(e.Get[component.TargetHistory]().Rows.Values()), row)
		e.Set(component.TargetHistory{Rows: cardinal.SliceOf(rows[max(len(rows)-keep, 0):]...)})
	}
}

// rowOf is target e as it is at tick, living all of it: its first shape's kind and circle at its origin,
// its side, and the tags its effects give it from the next tick on.
func rowOf(e cardinal.Entity, tick uint64) flight.Row {
	shape := e.Get[physics2d.PhysicsBody2D]().Shapes.At(0)
	return flight.Row{
		Tick:   tick,
		Kind:   interact.Kind(shape.CategoryBits),
		Side:   team.SideOf(e),
		Center: box2d.Vec2(e.Get[physics2d.Transform2D]().Position),
		Radius: shape.Radius,
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
