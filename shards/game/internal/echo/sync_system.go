package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/ref"
)

// syncRow is what players see of an Echo beyond its row.
type syncRow struct {
	Spawn   ref.Spawn
	Echo    component.Echo
	Health  combatcomp.Health
	Effects effectcomp.Effects
}

// SyncSystem (PostUpdate) sends each player the Echoes they may see, with SendTo. One
// that died this tick is sent once more, at HP 0: its row says when it died. O(E) to collect, then one
// send per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	var echoes netsync.Items[EchoState]
	next := w.TickHeight() + 1
	for e := range w.Contains[syncRow]().Iter() {
		h := e.Get[combatcomp.Health]()
		echoes.Add(e, netsync.Everyone, EchoState{
			EntityID: uint32(e.ID()),
			Serial:   e.Get[ref.Spawn]().Serial,
			PlayerID: e.Get[component.Echo]().PlayerID,
			Effects:  e.Get[effectcomp.Effects]().Shown(next),
			HP:       h.HP,
			MaxHP:    h.MaxHP,
		})
	}
	lists := echoes.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, EchoSync{Tick: w.TickHeight(), Echoes: lists.For(v)})
	}
}
