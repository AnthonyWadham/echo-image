package ability

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/ability/hooks"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/physics"
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
// cast's hook. The hook fills requests, and this turns them into intents with the cast's Cause, the
// caster's side and its position; the features that own the state act on them. O(C) for C casts.
type CastHookSystem struct {
	Data *data.Plugin
}

func (s *CastHookSystem) Run(w *cardinal.World) {
	tuning := data.Get[AbilityTuning](s.Data)
	casters := w.Contains[hookCasterRow]()
	for c := range w.SystemEvents[component.AbilityCast]() {
		hook, ok := tuning.Hook(c.Ability)
		if !ok || hook.OnCast == nil {
			continue
		}
		caster, err := casters.GetByID(c.Caster)
		if err != nil {
			continue // caster gone this tick
		}
		owner, side := caster.Get[team.Owner]().Body, caster.Get[team.Team]().ID
		pos := caster.Get[physics2d.Transform2D]().Position
		var out hooks.Out
		hook.OnCast(hooks.Cast{
			Caster: uint32(c.Caster), Owner: uint32(owner), Team: side,
			Pos: hooks.Vec2{X: pos.X, Y: pos.Y}, Aim: hooks.Vec2{X: c.AimX, Y: c.AimY},
			TickRate: physics.ServerTickRateHz,
		}, &out)
		for i, l := range out.Launches {
			w.EmitSystemEvent(projectilecomp.LaunchIntent{
				Shot: l.Shot, Cause: c.Cause, Launch: uint32(i), Owner: owner, Team: side,
				From: pos, Dir: physics2d.Vec2{X: l.Dir.X, Y: l.Dir.Y},
			})
		}
		for _, e := range out.Effects {
			w.EmitSystemEvent(effectcomp.EffectIntent(cast.EffectIntent(e, uint32(c.Caster))))
		}
	}
}
