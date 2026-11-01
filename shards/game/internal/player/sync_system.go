package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	abilitycomp "echo_image/shards/game/internal/ability/component"
	combatcomp "echo_image/shards/game/internal/combat/component"
	echocomp "echo_image/shards/game/internal/echo/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player/component"
	projectilecomp "echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
)

// syncRow is what players see of a Body.
type syncRow struct {
	Spawn     ref.Spawn
	Player    component.Player
	Champion  component.Champion
	Team      team.Team
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D // for its radius
	Effects   effectcomp.Effects
}

// shotRow is a shot in flight, for its predictor's YouState.
type shotRow struct {
	Projectile projectilecomp.Projectile
	Transform  physics2d.Transform2D
}

// echoRow is an Echo's path, for its Body's YouState.
type echoRow struct {
	Echo    echocomp.Echo
	Owner   team.Owner
	History echocomp.PathHistory
}

// SyncSystem (Update, after game logic) sends each player the Bodies they may see, and their own
// prediction data, with SendTo. O(B + P + E) to collect for B Bodies, P shots and E Echoes, then one
// send per player.
type SyncSystem struct {
	Tuning     MovementTuning
	SimVersion string // echoimage.SimVersion: the sim this shard runs
}

func (s *SyncSystem) Run(w *cardinal.World) {
	viewers := netsync.Viewers(w)
	if len(viewers) == 0 {
		return
	}
	var bodies netsync.Items[BodyState]
	next := w.TickHeight() + 1
	for e := range w.Contains[syncRow]().Iter() {
		p, fx := e.Get[component.Player](), e.Get[effectcomp.Effects]()
		pos := e.Get[physics2d.Transform2D]().Position
		bodies.Add(e, netsync.Everyone, BodyState{
			EntityID: uint32(e.ID()),
			Serial:   e.Get[ref.Spawn]().Serial,
			PlayerID: p.ID,
			Nickname: p.Nickname,
			Champion: e.Get[component.Champion]().ID,
			Team:     e.Get[team.Team]().ID,
			X:        pos.X,
			Y:        pos.Y,
			Radius:   radius(e),
			Tags:     fx.Status(next).Tags,
			Effects:  fx.Shown(next),
		})
	}
	shots := map[cardinal.EntityID][]ShotState{} // by the caster whose client predicts them
	for e := range w.Contains[shotRow]().Iter() {
		p := e.Get[projectilecomp.Projectile]()
		if !p.Predicted {
			continue
		}
		pos := e.Get[physics2d.Transform2D]().Position
		shots[p.Cause.Caster] = append(shots[p.Cause.Caster], ShotState{
			Type: p.Type, Ability: p.Cause.Ability, Seq: p.Cause.Seq, Launch: p.Launch,
			PrevX: p.Prev.X, PrevY: p.Prev.Y, X: pos.X, Y: pos.Y, VelX: p.Velocity.X, VelY: p.Velocity.Y,
			Radius: p.Radius, Remaining: p.Remaining,
		})
	}
	died := map[cardinal.EntityID]bool{}
	for d := range w.SystemEvents[combatcomp.Died]() {
		died[d.Target] = true
	}
	paths := map[cardinal.EntityID]echocomp.PathHistory{} // by owner Body, of Echoes still there next tick
	for e := range w.Contains[echoRow]().Iter() {
		if !died[e.ID()] {
			paths[e.Get[team.Owner]().Body] = e.Get[echocomp.PathHistory]()
		}
	}
	tuning := s.Tuning
	ahead := tuning.EchoPathTicks()
	lists := bodies.Lists(viewers)
	for _, v := range viewers {
		y := you(v.Body, shots[v.Body.ID()])
		y.InputCap = uint32(tuning.BufferFrames())
		y.InputTarget = uint32(v.Body.Get[component.InputQueue]().TargetAt(w.TickHeight(), tuning.QueueStepFrames(),
			tuning.QueueCalmTicks(), tuning.BufferFrames()))
		def := tuning.Champions[y.Champion]
		y.SnapSeconds, y.FadeSeconds = def.SnapSeconds, def.FadeSeconds
		if h, ok := paths[v.Body.ID()]; ok {
			y.EchoDelay, y.EchoPath = uint32(h.Delay()), cardinal.SliceOf(h.Path(ahead)...)
		}
		w.SendTo(v.Player, BodySync{
			Tick:        w.TickHeight(),
			TickSeconds: physics.SecondsPerTick,
			SimVersion:  s.SimVersion,
			You:         y,
			Bodies:      lists.For(v),
		})
	}
}

// you returns a viewer's own data, with the shots their client predicts.
func you(e cardinal.Entity, shots []ShotState) YouState {
	q := e.Get[component.InputQueue]()
	y := YouState{
		Body: uint32(e.ID()), Shots: cardinal.SliceOf(shots...),
		InputSession: q.Session, InputAck: q.Ack(), InputQueued: uint32(q.Pending.Len()),
		MoveSpeed: e.Get[component.MoveSpeed]().Value, Champion: e.Get[component.Champion]().ID,
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
