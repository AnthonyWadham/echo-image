package projectile

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/internal/team"
)

// FlightSystem (Update, after LaunchSystem, before combat.DamageResolveSystem) moves every shot one
// tick along its path with flight.Fly, the code the client predictor flies its own shots with, and
// resolves the first thing on it that the shot doesn't pass. Rules decide what a shot does on meeting
// each thing: the shot type's own, then the defaults (GDD §7). It passes the thing or stops there, and
// a payload rule gives the thing its damage as a DamageIntent. A shot launched this tick moves this
// tick, so its path starts at the shooter. One Box2D shape cast per shot finds the map, Bodies and
// Echoes on its path, so the cost is O(P·(log N + K·R)) for P shots, N shapes, K found and R rules.
type FlightSystem struct {
	Data    *data.Plugin
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
	tuning := data.Get[ProjectileTuning](s.Data)
	world := s.Physics.Engine()
	targets := targetsOf(w, world)
	// Resolved after the search finished, since a resolved shot is destroyed.
	for _, shot := range shots {
		fly(w, world, tuning, shot, targets)
	}
}

func fly(w *cardinal.World, world *box2d.World, tuning ProjectileTuning, shot cardinal.Entity, targets flight.Targets) {
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
	next, over, hit := tuning.Fly(world, def, f, team.SideOf(shot), targets, physics.SecondsPerTick)
	if !over {
		p.Remaining = next.Remaining
		shot.Set(p)
		shot.Set(physics2d.Transform2D{Position: physics2d.Vec2(next.Pos)})
		return
	}
	if hit.Meet.Payload {
		w.EmitSystemEvent(combatcomp.DamageIntent{
			Target: cardinal.EntityID(hit.Target), Credit: shot.Get[team.Owner]().Body, Cause: p.Cause,
			Amount: p.Damage,
		})
	}
	// DEBUG(shots): temporary, remove after the off-screen shot hunt.
	w.Logger().Info().Uint32("shot", uint32(shot.ID())).Uint32("caster", uint32(p.Cause.Caster)).
		Float64("x", f.Pos.X).Float64("y", f.Pos.Y).Float64("remaining", f.Remaining).
		Uint32("target", hit.Target).Bool("payload", hit.Meet.Payload).Msg("DEBUG shot ended")
	shot.Destroy()
}

// targetsOf says whose each Body and Echo shape in world is, with its side and its tags this tick.
func targetsOf(w *cardinal.World, world *box2d.World) flight.Targets {
	tick := w.TickHeight()
	return func(shape box2d.ShapeID) (flight.Target, bool) {
		e := w.Entity(physics.EntityOf(world, shape))
		if !e.Has[team.Team]() || !e.Has[team.Owner]() || !e.Has[effectcomp.Effects]() {
			return flight.Target{}, false
		}
		return flight.Target{
			ID:   uint32(e.ID()),
			Side: team.SideOf(e),
			Tags: e.Get[effectcomp.Effects]().Status(tick).Tags,
		}, true
	}
}
