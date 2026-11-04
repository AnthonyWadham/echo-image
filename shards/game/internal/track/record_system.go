package track

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/team"
	"echo_image/shards/game/internal/track/component"
	"echo_image/shards/game/internal/track/past"
)

// recordRow is a thing that keeps its track.
type recordRow struct {
	Track     component.Track
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D
	Team      team.Team
	Owner     team.Owner
}

// RecordSystem (Update, after effect.ResolveSystem, before the syncs) records this tick's row of every
// tracked thing, which is what this tick's syncs show of it. Judges meet it by its rows, at the view
// their client drew, and SyncSystem sends them. It keeps those of every view the window allows
// (KeptRows), each written in place: O(T) for T things.
type RecordSystem struct {
	Window past.Window
}

func (s *RecordSystem) Run(w *cardinal.World) {
	keep := int(s.Window.KeptRows())
	tick := w.TickHeight()
	died := map[cardinal.EntityID]float64{}
	for d := range w.SystemEvents[combatcomp.Died]() {
		died[d.Target] = d.Until
	}
	for e := range w.Contains[recordRow]().Iter() {
		if e.Get[physics2d.PhysicsBody2D]().Shapes.Len() == 0 {
			continue
		}
		row := rowOf(e, tick)
		if until, ok := died[e.ID()]; ok {
			row.Until = until
		}
		e.Set(e.Get[component.Track]().Record(row, keep))
	}
}

// rowOf is thing e as it is at tick, living all of it: its first shape's kind and circle at its origin,
// its side, and the tags its effects give it from the next tick on.
func rowOf(e cardinal.Entity, tick uint64) past.Row {
	shape := e.Get[physics2d.PhysicsBody2D]().Shapes.At(0)
	return past.Row{
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
