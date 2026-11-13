package player

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/pkg/levelstate"
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

// mapRow is the live map.
type mapRow struct {
	Current levelstate.Current
}

// echoRow is an Echo's path, for its Body's YouState.
type echoRow struct {
	Spawn   ref.Spawn
	Echo    echocomp.Echo
	Owner   team.Owner
	History echocomp.PathHistory
}

// SyncSystem (PostUpdate) sends each player the Bodies they may see, and their own
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
		p := e.Get[component.Player]()
		bodies.Add(e, netsync.Everyone, BodyState{
			EntityID: uint32(e.ID()),
			Serial:   e.Get[ref.Spawn]().Serial,
			PlayerID: p.ID,
			Nickname: p.Nickname,
			Champion: e.Get[component.Champion]().ID,
			Effects:  e.Get[effectcomp.Effects]().Shown(next),
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
	echoes := map[cardinal.EntityID]cardinal.Entity{} // by owner Body, of Echoes still there next tick
	for e := range w.Contains[echoRow]().Iter() {
		if !died[e.ID()] {
			echoes[e.Get[team.Owner]().Body] = e
		}
	}
	var mapID string
	for e := range w.Contains[mapRow]().Iter() {
		mapID = e.Get[levelstate.Current]().ID
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
		if e, ok := echoes[v.Body.ID()]; ok {
			h := e.Get[echocomp.PathHistory]()
			y.EchoDelay, y.EchoPath = uint32(h.Delay()), cardinal.SliceOf(h.Path(ahead)...)
			y.EchoSerial = e.Get[ref.Spawn]().Serial
		}
		w.SendTo(v.Player, BodySync{
			Tick:        w.TickHeight(),
			TickSeconds: physics.SecondsPerTick,
			SimVersion:  s.SimVersion,
			MapID:       mapID,
			You:         y,
			Bodies:      lists.For(v),
		})
	}
}

// you returns a viewer's own data, with the shots their client predicts.
func you(e cardinal.Entity, shots []ShotState) YouState {
	q, pos := e.Get[component.InputQueue](), e.Get[physics2d.Transform2D]().Position
	y := YouState{
		Body: uint32(e.ID()), Team: e.Get[team.Team]().ID, X: pos.X, Y: pos.Y, Radius: radius(e),
		Shots:        cardinal.SliceOf(shots...),
		InputSession: q.Session, InputAck: q.Ack(), InputQueued: uint32(q.Pending.Len()), InputHealth: q.Health,
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
