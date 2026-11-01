package projectile

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/internal/team"
)

// FlightSystem (Update, after LaunchSystem, before combat.DamageResolveSystem) moves every shot one
// tick along its path with flight.Fly, the code the client predictor flies its own shots with, and
// resolves the first thing on it that the shot doesn't pass. Rules decide what a shot does on meeting
// each thing: the shot type's own, then the defaults (GDD §7). It passes the thing or stops there, and
// a payload rule gives the thing its damage as a DamageIntent. Every end, its range included, is a
// component.ShotEnded: where, when in the tick and on what. A shot launched this tick moves this tick,
// so its path starts at the shooter.
//
// A shot an input frame cast meets the map now, and a Body or Echo only through a hit claim in its
// caster's frame applied this tick: flight.Confirm checks it where the client drew them, from their
// TargetHistory. A shot no frame cast (an Echo's) meets them where they are now. One Box2D shape cast
// per shot finds what it meets, so the cost is O(P·(log N + K·R)) for P shots, N shapes, K found and R
// rules, plus O(log N + K) per claim.
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
	side := team.SideOf(shot)
	meets := targets
	if p.Cause.Seq != 0 {
		meets = nil // the map only
	}
	next, end, over := tuning.Fly(world, def, f, side, meets, physics.SecondsPerTick)
	if claim, ok := claimed(w, world, tuning, p, def, f, side); ok {
		end, over = claim, true
	}
	if !over {
		p.Remaining = next.Remaining
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
		Shot: shot.ID(), Type: p.Type, Cause: p.Cause, Team: shot.Get[team.Team]().ID, Radius: p.Radius, End: end,
	})
	shot.Destroy()
}

// claimed checks the hit claim on shot p, flying f this tick, in the frame its caster applied this
// tick. It returns how the shot ends, or false with no claim or a rejected one. A frame names a shot by
// its key, and the first claim on it counts.
func claimed(w *cardinal.World, world *box2d.World, tuning ProjectileTuning, p component.Projectile, def flight.Shot,
	f flight.Flight, side interact.Side,
) (flight.End, bool) {
	tick := w.TickHeight()
	caster := w.Entity(p.Cause.Caster)
	if p.Cause.Seq == 0 || !caster.Has[playercomp.InputQueue]() {
		return flight.End{}, false
	}
	frame, ok := caster.Get[playercomp.InputQueue]().Applied(tick)
	if !ok || !tuning.ValidView(frame.ViewTick, tick) {
		return flight.End{}, false
	}
	i := frame.Claims.IndexFunc(func(c playercomp.HitClaim) bool {
		return c.Seq == p.Cause.Seq && c.Ability == p.Cause.Ability && c.Launch == p.Launch
	})
	if i < 0 {
		return flight.End{}, false
	}
	view := frame.ViewTick
	target, ok := seenAt(w.Entity(cardinal.EntityID(frame.Claims.At(i).Target)), caster.ID(), view, tick)
	if !ok {
		return flight.End{}, false
	}
	sights := func(shape box2d.ShapeID) (flight.Seen, bool) {
		return seenAt(w.Entity(physics.EntityOf(world, shape)), caster.ID(), view, tick)
	}
	return tuning.Confirm(world, def, f, side, target, float64(tick)-view, sights, physics.SecondsPerTick)
}

// seenAt is e where a claim's view had it, from its TargetHistory, or false when e isn't a Body or Echo
// with rows then. The caster's own Body is where it is now, as its client predicts it.
func seenAt(e cardinal.Entity, caster cardinal.EntityID, view float64, tick uint64) (flight.Seen, bool) {
	if !e.Has[component.TargetHistory]() || !e.Has[physics2d.PhysicsBody2D]() || !e.Has[team.Team]() ||
		!e.Has[team.Owner]() {
		return flight.Seen{}, false
	}
	shapes := e.Get[physics2d.PhysicsBody2D]().Shapes
	if shapes.Len() == 0 {
		return flight.Seen{}, false
	}
	s := flight.Seen{
		Target: flight.Target{ID: uint32(e.ID()), Side: team.SideOf(e)},
		Kind:   interact.Kind(shapes.At(0).CategoryBits) & (interact.Body | interact.Echo),
	}
	if e.ID() == caster {
		s.Center, s.Radius, s.Tags = box2d.Vec2(e.Get[physics2d.Transform2D]().Position), shapes.At(0).Radius, tagsAt(e, tick)
		return s, s.Kind != 0
	}
	history := e.Get[component.TargetHistory]()
	a := uint64(math.Floor(view))
	newer, ok := history.At(a + 1)
	if !ok {
		return flight.Seen{}, false
	}
	older, hasOlder := history.At(a)
	row, ok := flight.AtView(older, hasOlder, newer, view)
	s.Center, s.Radius, s.Tags = row.Center, row.Radius, row.Tags
	return s, ok && s.Kind != 0
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
