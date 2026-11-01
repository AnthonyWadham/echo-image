package ability

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/ability/hooks"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/effect/status"
	projectilecomp "echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/team"
)

// hookCasterRow is a caster: the side it acts for, and where it is.
type hookCasterRow struct {
	Team      team.Team
	Owner     team.Owner
	Transform physics2d.Transform2D
}

// CastHookSystem (Update, after echo.ReplaySystem and before the receivers of its intents) runs each
// cast's hook (cast.Run) and emits what it asks for as intents, with the cast's Cause and the caster's
// side; the features that own the state act on them. O(C) for C casts.
type CastHookSystem struct {
	Tuning AbilityTuning
}

func (s *CastHookSystem) Run(w *cardinal.World) {
	tuning := s.Tuning
	casters := w.Contains[hookCasterRow]()
	for c := range w.SystemEvents[component.AbilityCast]() {
		caster, err := casters.GetByID(c.Caster)
		if err != nil {
			continue // caster gone this tick
		}
		owner, side := caster.Get[team.Owner]().Body, caster.Get[team.Team]().ID
		pos := caster.Get[physics2d.Transform2D]().Position
		tuning.Run(c.Ability, hooks.Cast{
			Caster: uint32(c.Caster), Owner: uint32(owner), Team: side,
			Pos: hooks.Vec2{X: pos.X, Y: pos.Y}, Aim: hooks.Vec2{X: c.AimX, Y: c.AimY},
		}, intents{w: w, cause: c.Cause, owner: owner, team: side})
	}
}

// intents is a cast.Sink that emits one cast's requests as intents.
type intents struct {
	w     *cardinal.World
	cause component.Cause
	owner cardinal.EntityID
	team  uint8
}

func (s intents) Launch(l cast.Launch) {
	s.w.EmitSystemEvent(projectilecomp.LaunchIntent{
		Shot: l.Shot, Cause: s.cause, Launch: l.Launch, Owner: s.owner, Team: s.team,
		From: physics2d.Vec2{X: l.From.X, Y: l.From.Y}, Dir: physics2d.Vec2{X: l.Dir.X, Y: l.Dir.Y},
	})
}

func (s intents) Effect(in status.Intent) { s.w.EmitSystemEvent(effectcomp.EffectIntent(in)) }
