package projectile

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/internal/team"
)

// targetRow is an entity whose Body or Echo circles a shot can meet.
type targetRow struct {
	Team      team.Team
	Owner     team.Owner
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D
	Effects   effectcomp.Effects
}

// FlightSystem (Update, after LaunchSystem, before combat.DamageResolveSystem) moves every shot one
// tick along its path with flight.Fly, the code the client predictor flies its own shots with, and
// resolves the first thing on it that the shot doesn't pass. Rules decide what a shot does on meeting
// each thing: the shot type's own, then the defaults (GDD §7). It passes the thing or stops there, and
// a payload rule gives the thing its damage as a DamageIntent. A shot launched this tick moves this
// tick, so its path starts at the shooter. O(P·T·R) for P shots, T Body/Echo circles and R rules,
// plus one Box2D shape cast per shot.
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
	targets := collectTargets(w)
	world := s.Physics.Engine()
	// Resolved after the search finished, since a resolved shot is destroyed.
	for _, shot := range shots {
		fly(w, world, tuning, shot, targets)
	}
}

func fly(w *cardinal.World, world *box2d.World, tuning ProjectileTuning, shot cardinal.Entity, targets []flight.Target) {
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

// collectTargets lists every Body and Echo circle once per tick, so each shot tests plain values.
func collectTargets(w *cardinal.World) []flight.Target {
	tick := w.TickHeight()
	var targets []flight.Target
	for e := range w.Contains[targetRow]().Iter() {
		pose := e.Get[physics2d.Transform2D]()
		side := team.SideOf(e)
		tags := e.Get[effectcomp.Effects]().Status(tick).Tags
		for shape := range e.Get[physics2d.PhysicsBody2D]().Shapes.Values() {
			kind := interact.Kind(shape.CategoryBits) & (interact.Body | interact.Echo)
			if kind == 0 || shape.ShapeType != physics2d.ShapeTypeCircle {
				continue
			}
			targets = append(targets, flight.Target{
				ID:     uint32(e.ID()),
				Kind:   kind,
				Side:   side,
				Tags:   tags,
				Center: box2d.Vec2(physics.WorldPoint(pose, shape.LocalOffset)),
				Radius: shape.Radius,
			})
		}
	}
	return targets
}
