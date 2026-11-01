package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/team"
)

// syncRow is what players see of an Echo.
type syncRow struct {
	Echo      component.Echo
	Health    combatcomp.Health
	Team      team.Team
	Owner     team.Owner
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D // for its radius
	Effects   effectcomp.Effects      // for its tags
}

// SyncSystem (Update, after damage resolves) sends each player the Echoes they may see, with SendTo. One
// that died this tick (combatcomp.Died) is sent once more, at HP 0, with when it died. O(E + D) to
// collect, then one send per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	died := map[cardinal.EntityID]float64{}
	for d := range w.SystemEvents[combatcomp.Died]() {
		died[d.Target] = d.Until
	}
	var echoes netsync.Items[EchoState]
	for e := range w.Contains[syncRow]().Iter() {
		until, dead := died[e.ID()]
		if !dead {
			until = 1
		}
		h := e.Get[combatcomp.Health]()
		pos := e.Get[physics2d.Transform2D]().Position
		echoes.Add(e, netsync.Everyone, EchoState{
			EntityID: uint32(e.ID()),
			PlayerID: e.Get[component.Echo]().PlayerID,
			Owner:    uint32(e.Get[team.Owner]().Body),
			Team:     e.Get[team.Team]().ID,
			X:        pos.X,
			Y:        pos.Y,
			Radius:   e.Get[physics2d.PhysicsBody2D]().Shapes.At(0).Radius,
			Tags:     e.Get[effectcomp.Effects]().Status(w.TickHeight() + 1).Tags,
			HP:       h.HP,
			MaxHP:    h.MaxHP,
			Until:    until,
		})
	}
	lists := echoes.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, EchoSync{Tick: w.TickHeight(), Echoes: lists.For(v)})
	}
}
