package history

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/history/component"
	"echo_image/shards/game/internal/history/past"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/team"
)

// recordRow is a thing that keeps its history.
type recordRow struct {
	History   component.History
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D
	Team      team.Team
	Owner     team.Owner
}

// RecordSystem (PostUpdate, first, so after every writer) records this tick's row of every
// recorded thing, which is what this tick's syncs show of it. Judges meet it by its rows, at the view
// their client drew, and SyncSystem sends them. It keeps those of every view the window allows
// (KeptRows), each written in place: O(T log D + D log D) for T things and D deaths.
type RecordSystem struct {
	Window past.Window
	died   []combatcomp.Died // scratch: this tick's deaths, by target
}

func (s *RecordSystem) Run(w *cardinal.World) {
	keep := int(s.Window.KeptRows())
	tick := w.TickHeight()
	s.died = slices.AppendSeq(s.died[:0], w.SystemEvents[combatcomp.Died]())
	slices.SortFunc(s.died, func(a, b combatcomp.Died) int { return cmp.Compare(a.Target, b.Target) })
	for e := range w.Contains[recordRow]().Iter() {
		if e.Get[physics2d.PhysicsBody2D]().Shapes.Len() == 0 {
			continue
		}
		row := rowOf(e, tick)
		if i, ok := slices.BinarySearchFunc(s.died, e.ID(), func(d combatcomp.Died, id cardinal.EntityID) int {
			return cmp.Compare(d.Target, id)
		}); ok {
			row.Until = s.died[i].Until
		}
		e.Set(e.Get[component.History]().Record(row, keep))
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
		Tags:   effectcomp.TagsOf(e, tick+1),
		Until:  1,
	}
}
