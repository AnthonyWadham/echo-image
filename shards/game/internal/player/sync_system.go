package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	abilitycomp "echo_image/shards/game/internal/ability/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player/component"
	projectilecomp "echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/team"
)

// syncRow is what players see of a Body. Movement components are read only for the owner's
// YouState, so a Body without them is still shown.
type syncRow struct {
	Player    component.Player
	Team      team.Team
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D // for its radius
}

// shotRow is a shot in flight, for its caster's YouState.
type shotRow struct {
	Projectile projectilecomp.Projectile
	Transform  physics2d.Transform2D
}

// SyncSystem (Update, after game logic) sends each player the Bodies they may see, and their own
// prediction data, with SendTo. O(B + P) to collect for B Bodies and P shots, then one send per player.
type SyncSystem struct{}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	var bodies netsync.Items[BodyState]
	for e := range w.Contains[syncRow]().Iter() {
		p := e.Get[component.Player]()
		pos := e.Get[physics2d.Transform2D]().Position
		bodies.Add(e, netsync.Everyone, BodyState{
			EntityID: uint32(e.ID()),
			PlayerID: p.ID,
			Nickname: p.Nickname,
			Team:     e.Get[team.Team]().ID,
			X:        pos.X,
			Y:        pos.Y,
			Radius:   radius(e),
		})
	}
	shots := map[cardinal.EntityID][]ShotState{} // by caster
	for e := range w.Contains[shotRow]().Iter() {
		p := e.Get[projectilecomp.Projectile]()
		pos := e.Get[physics2d.Transform2D]().Position
		shots[p.Cause.Caster] = append(shots[p.Cause.Caster], ShotState{
			Type: p.Type, X: pos.X, Y: pos.Y, VelX: p.Velocity.X, VelY: p.Velocity.Y,
			Radius: p.Radius, Remaining: p.Remaining,
		})
	}
	lists := bodies.Lists(viewers)
	for _, v := range viewers {
		w.SendTo(v.Player, BodySync{
			Tick:        w.TickHeight(),
			TickSeconds: physics.SecondsPerTick,
			You:         you(v.Body, shots[v.Body.ID()]),
			Bodies:      lists.For(v),
		})
	}
}

// you returns a viewer's own data, with the shots their Body cast. A Body without movement
// components gets zero ack and speed.
func you(e cardinal.Entity, shots []ShotState) YouState {
	y := YouState{Body: uint32(e.ID()), Shots: cardinal.SliceOf(shots...)}
	if e.Has[component.InputQueue]() {
		q := e.Get[component.InputQueue]()
		y.InputSession, y.InputAck = q.Session, q.Ack()
	}
	if e.Has[component.MoveSpeed]() {
		y.MoveSpeed = e.Get[component.MoveSpeed]().Value
	}
	if e.Has[component.Champion]() {
		y.Champion = e.Get[component.Champion]().ID
	}
	if e.Has[abilitycomp.Abilities]() {
		y.Cooldowns = e.Get[abilitycomp.Abilities]().Cooldowns
	}
	if e.Has[effectcomp.Effects]() {
		fx := e.Get[effectcomp.Effects]()
		y.Effects, y.EffectsNext = fx.List, fx.Next
	}
	return y
}
