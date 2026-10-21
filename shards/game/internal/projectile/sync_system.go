package projectile

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/team"
)

// SyncSystem (Update, after shots fly) sends each player the shots they may see, with SendTo. O(P) to
// collect, then one send per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	var shots netsync.Items[ProjectileState]
	for e := range w.Contains[projectileRow]().Iter() {
		pos := e.Get[physics2d.Transform2D]().Position
		shots.Add(e, netsync.Everyone, ProjectileState{
			EntityID: uint32(e.ID()),
			Team:     e.Get[team.Team]().ID,
			X:        pos.X,
			Y:        pos.Y,
			Radius:   e.Get[component.Projectile]().Radius,
		})
	}
	lists := shots.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, ProjectileSync{Tick: w.TickHeight(), Projectiles: lists.For(v)})
	}
}
