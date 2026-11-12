package history

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/history/component"
	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/ref"
)

// syncRow is a recorded thing players may be sent.
type syncRow struct {
	Spawn   ref.Spawn
	History component.History
}

// SyncSystem (PostUpdate, after RecordSystem) sends each player the recorded things they
// may see, each as the row RecordSystem recorded this tick, so their screen draws it and their predictor
// places it as the server's judges do. O(T) to collect, then one send per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	tick := w.TickHeight()
	var targets netsync.Items[TargetState]
	for e := range w.Contains[syncRow]().Iter() {
		r, ok := e.Get[component.History]().At(tick)
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
