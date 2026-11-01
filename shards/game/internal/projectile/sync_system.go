package projectile

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/team"
)

// SyncSystem (Update, after shots fly) sends each player the shots they may see, with SendTo: those in
// flight, and each component.ShotEnded of the tick, where and when it ended, so a client draws it to
// its end. O(P + E) to collect, then one send per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	var shots netsync.Items[ProjectileState]
	for e := range w.Contains[projectileRow]().Iter() {
		pos := e.Get[physics2d.Transform2D]().Position
		p := e.Get[component.Projectile]()
		shots.Add(e, netsync.Everyone, ProjectileState{
			EntityID: uint32(e.ID()),
			Type:     p.Type,
			Caster:   uint32(p.Cause.Caster),
			Team:     e.Get[team.Team]().ID,
			X:        pos.X,
			Y:        pos.Y,
			Radius:   p.Radius,
			Until:    1,
		})
	}
	for ev := range w.SystemEvents[component.ShotEnded]() {
		shots.AddGone(ev.Team, netsync.Everyone, ProjectileState{
			EntityID: uint32(ev.Shot), Type: ev.Type, Caster: uint32(ev.Cause.Caster), Team: ev.Team,
			X: ev.End.At.X, Y: ev.End.At.Y, Radius: ev.Radius, Until: ev.End.Until, Ended: true,
		})
	}
	lists := shots.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, ProjectileSync{Tick: w.TickHeight(), Projectiles: lists.For(v)})
	}
}
