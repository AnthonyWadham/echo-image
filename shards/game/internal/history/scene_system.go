package history

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/history/component"
	"echo_image/shards/game/internal/history/past"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
)

// sceneRow is a recorded thing a scene holds.
type sceneRow struct {
	Spawn     ref.Spawn
	History   component.History
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D
	Team      team.Team
	Owner     team.Owner
}

// clientRow is a Body a client plays.
type clientRow struct {
	Spawn ref.Spawn
	Input playercomp.InputQueue
}

// SceneSystem (Update, after echo.SpawnSystem, before every judge) builds this tick's Scene: each
// recorded thing as it is now, with its tags now, and its kept rows, and each client's view. A thing
// moves from its row a tick ago to now, as its owner's client draws it if it predicts it. Nothing
// between it and the judges moves a recorded thing. O(T·W + T log T + P log P).
type SceneSystem struct {
	Scene *past.Scene
	kept  []past.Row
}

func (s *SceneSystem) Run(w *cardinal.World) {
	tick := w.TickHeight()
	s.Scene.Reset(tick)
	for e := range w.Contains[sceneRow]().Iter() {
		if e.Get[physics2d.PhysicsBody2D]().Shapes.Len() == 0 {
			continue
		}
		tr, now := e.Get[component.History](), rowOf(e, tick)
		now.Tags = effectcomp.TagsOf(e, tick)
		from := now.Center
		if prev, ok := tr.At(tick - 1); ok {
			from = prev.Center
		}
		s.kept = tr.AppendRows(s.kept[:0], tick)
		s.Scene.Add(past.Thing{ID: uint32(e.ID()), Serial: e.Get[ref.Spawn]().Serial, Now: now,
			Predicted: tr.Predicted, From: from, To: now.Center}, s.kept)
	}
	for e := range w.Contains[clientRow]().Iter() {
		if view, ok := e.Get[playercomp.InputQueue]().View(tick); ok {
			s.Scene.AddClient(uint32(e.ID()), e.Get[ref.Spawn]().Serial, view)
		}
	}
	s.Scene.Build()
}
