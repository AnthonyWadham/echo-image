package projectile

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/ref"
)

// targetSyncRow is a target players may be sent.
type targetSyncRow struct {
	Spawn   ref.Spawn
	History component.TargetHistory
}

// TargetSyncSystem (Update, after HistorySystem) sends each player the targets they may see, each as the
// row HistorySystem saved this tick, so their predictor places them as FlightSystem does. O(T) to
// collect, then one send per player.
type TargetSyncSystem struct{}

func (s *TargetSyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	tick := w.TickHeight()
	var targets netsync.Items[TargetState]
	for e := range w.Contains[targetSyncRow]().Iter() {
		r, ok := e.Get[component.TargetHistory]().At(tick)
		if !ok {
			continue
		}
		targets.Add(e, netsync.Everyone, TargetState{
			EntityID: uint32(e.ID()), Serial: e.Get[ref.Spawn]().Serial, Kind: r.Kind, Owner: r.Side.Owner,
			Team: r.Side.Team, X: r.Center.X, Y: r.Center.Y, Radius: r.Radius, Tags: r.Tags, Until: r.Until,
		})
	}
	lists := targets.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, TargetSync{Tick: tick, Targets: lists.For(v)})
	}
}
