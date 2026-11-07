package projectile

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
)

// SyncSystem (PostUpdate) sends each player the shots they may see, with SendTo: those in
// flight, and each component.ShotEnded of the tick, where and when it ended, so a client draws it to
// its end. A player's own predicted shots are left out: its predictor draws them. O(P + E) to collect,
// then O(P + E) per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	var shots netsync.Items[ProjectileState]
	predictor := map[uint64]cardinal.EntityID{} // by serial: the Body whose client predicts the shot
	for e := range w.Contains[projectileRow]().Iter() {
		pos := e.Get[physics2d.Transform2D]().Position
		p := e.Get[component.Projectile]()
		serial := e.Get[ref.Spawn]().Serial
		if p.Predicted {
			predictor[serial] = p.Cause.Caster
		}
		shots.Add(e, netsync.Everyone, ProjectileState{
			EntityID: uint32(e.ID()),
			Serial:   serial,
			Type:     p.Type,
			Caster:   uint32(p.Cause.Caster),
			Team:     e.Get[team.Team]().ID,
			PrevX:    p.Prev.X,
			PrevY:    p.Prev.Y,
			X:        pos.X,
			Y:        pos.Y,
			Radius:   p.Radius,
			Until:    1,
		}, w.Entity(p.Cause.Caster)) // its first tick starts at its caster
	}
	for ev := range w.SystemEvents[component.ShotEnded]() {
		if ev.Predicted {
			predictor[ev.Serial] = ev.Cause.Caster
		}
		hit := ev.End.Hit
		reveals := []cardinal.Entity{w.Entity(ev.Cause.Caster)}
		if hit.OnTarget() {
			reveals = append(reveals, w.Entity(cardinal.EntityID(hit.Target))) // end minus offset is its center
		}
		last := netsync.Last{Team: ev.Team, X: ev.End.At.X, Y: ev.End.At.Y}
		shots.AddGone(last, netsync.Everyone, ProjectileState{
			EntityID: uint32(ev.Shot), Serial: ev.Serial, Type: ev.Type, Caster: uint32(ev.Cause.Caster),
			Team: ev.Team, PrevX: ev.Prev.X, PrevY: ev.Prev.Y, X: ev.End.At.X, Y: ev.End.At.Y,
			Radius: ev.Radius, Until: ev.End.Until, Ended: true,
			OnTarget: hit.OnTarget(), TargetSerial: hit.Serial, OffsetX: hit.Offset.X, OffsetY: hit.Offset.Y,
		}, reveals...)
	}
	lists := shots.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, ProjectileSync{Tick: w.TickHeight(), Projectiles: notPredictedBy(lists.For(v), predictor, v.Body.ID())})
	}
}

// notPredictedBy is list without the shots body's client predicts.
func notPredictedBy(list cardinal.Slice[ProjectileState], predictor map[uint64]cardinal.EntityID,
	body cardinal.EntityID,
) cardinal.Slice[ProjectileState] {
	var out []ProjectileState
	for s := range list.Values() {
		if by, ok := predictor[s.Serial]; !ok || by != body {
			out = append(out, s)
		}
	}
	return cardinal.SliceOf(out...)
}
