package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo/component"
	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/team"
)

// syncRow is what players see of an Echo.
type syncRow struct {
	Echo      component.Echo
	Health    combatcomp.Health
	Team      team.Team
	Transform physics2d.Transform2D
}

// SyncSystem (Update, after damage resolves) sends each player the Echoes they may see, with SendTo.
// O(E) to collect, then one send per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	var echoes netsync.Items[EchoState]
	for e := range w.Contains[syncRow]().Iter() {
		h := e.Get[combatcomp.Health]()
		pos := e.Get[physics2d.Transform2D]().Position
		echoes.Add(e, netsync.Everyone, EchoState{
			EntityID: uint32(e.ID()),
			PlayerID: e.Get[component.Echo]().PlayerID,
			Team:     e.Get[team.Team]().ID,
			X:        pos.X,
			Y:        pos.Y,
			HP:       h.HP,
			MaxHP:    h.MaxHP,
		})
	}
	lists := echoes.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, EchoSync{Tick: w.TickHeight(), Echoes: lists.For(v)})
	}
}
