package projectile

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
	"echo_image/shards/game/internal/track/past"
)

// FlightSystem (Update, after LaunchSystem, before combat.DamageResolveSystem) moves every shot one
// tick along its path with flight.Fly, the code the client predictor flies its own shots with, and
// resolves the first thing on it that the shot doesn't pass. Rules decide what a shot does on meeting
// each thing: the shot type's own, then the defaults (GDD §7). It passes the thing or stops there, and
// a payload rule gives the thing its damage as a DamageIntent. Every end, its range included, is a
// component.ShotEnded: where, when in the tick and on what. A shot launched this tick moves this tick,
// so its path starts at the shooter.
//
// A shot meets the map now, and each tracked thing as its actor saw it: a predicted shot at its caster's
// view, while that caster is still the one its Cause names, any other as screens draw this tick, as the
// client's predictor does. O(P·(log N + K log K + K·R)) for P shots, N shapes, K things, R rules.
type FlightSystem struct {
	Tuning  ProjectileTuning
	Scene   *past.Scene // this tick's tracked things, as track.SceneSystem built it
	Physics *physics2d.Plugin
}

func (s *FlightSystem) Run(w *cardinal.World) {
	var shots []cardinal.Entity
	for e := range w.Contains[projectileRow]().Iter() {
		shots = append(shots, e)
	}
	if len(shots) == 0 {
		return
	}
	world := s.Physics.Engine()
	// Resolved after the search finished, since a resolved shot is destroyed.
	for _, shot := range shots {
		fly(w, world, s.Tuning, s.Scene, shot)
	}
}

func fly(w *cardinal.World, world *box2d.World, tuning ProjectileTuning, scene *past.Scene, shot cardinal.Entity) {
	p := shot.Get[component.Projectile]()
	def, ok := tuning.Shots[p.Type]
	if !ok {
		w.Logger().Error().Str("type", p.Type).Msg("removed a shot of unknown type")
		shot.Destroy()
		return
	}
	f := flight.Flight{
		Pos:       box2d.Vec2(shot.Get[physics2d.Transform2D]().Position),
		Velocity:  box2d.Vec2(p.Velocity),
		Radius:    p.Radius,
		Remaining: p.Remaining,
	}
	side := team.SideOf(shot)
	a, ok := scene.Client(uint32(p.Cause.Caster), p.Cause.CasterSerial, side)
	if !p.Predicted || !ok {
		a = scene.Present(side)
	}
	next, end, over := tuning.Fly(world, def, f, a, scene, physics.SecondsPerTick)
	if !over {
		p.Remaining, p.Prev = next.Remaining, physics2d.Vec2(f.Pos)
		shot.Set(p)
		shot.Set(physics2d.Transform2D{Position: physics2d.Vec2(next.Pos)})
		return
	}
	if end.Hit.Meet.Payload {
		w.EmitSystemEvent(combatcomp.DamageIntent{
			Target: cardinal.EntityID(end.Hit.Target), Credit: shot.Get[team.Owner]().Body, Cause: p.Cause,
			Amount: p.Damage, Until: end.Until,
		})
	}
	w.EmitSystemEvent(component.ShotEnded{
		Shot: shot.ID(), Serial: shot.Get[ref.Spawn]().Serial, Type: p.Type, Cause: p.Cause, Predicted: p.Predicted,
		Team: shot.Get[team.Team]().ID, Radius: p.Radius, Prev: f.Pos, End: end,
	})
	shot.Destroy()
}
