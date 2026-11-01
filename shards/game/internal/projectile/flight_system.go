package projectile

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/internal/ref"
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
// A predicted shot meets the map now, and each Body and Echo where its caster's client drew it: at the
// view of the caster's frame this tick, from their TargetHistory (flight.FlyAtView). That is what the
// client's predictor flies it against, so a hit on screen is a hit on the server. Any other shot (an
// Echo's) meets them where they are now. One Box2D cast or tree query per shot finds what it meets, so
// the cost is O(P·(log N + K·R)) for P shots, N shapes, K found and R rules, plus O(T·W) a tick for the
// tree of T targets keeping W rows.
type FlightSystem struct {
	Tuning  ProjectileTuning
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
	tuning := s.Tuning
	world := s.Physics.Engine()
	targets, reach := targetsOf(w, world), reachOf(w)
	// Resolved after the search finished, since a resolved shot is destroyed.
	for _, shot := range shots {
		fly(w, world, tuning, shot, targets, reach)
	}
}

func fly(w *cardinal.World, world *box2d.World, tuning ProjectileTuning, shot cardinal.Entity, targets flight.Targets,
	reach *flight.Reach,
) {
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
	var next flight.Flight
	var end flight.End
	var over bool
	if view, ok := viewOf(w, tuning, p); ok {
		tick := w.TickHeight()
		sights := func(id uint32) (flight.Seen, bool) {
			return seenAt(w.Entity(cardinal.EntityID(id)), p.Cause.Caster, view, tick)
		}
		next, end, over = tuning.FlyAtView(world, def, f, side, reach, sights, physics.SecondsPerTick)
	} else {
		next, end, over = tuning.Fly(world, def, f, side, targets, physics.SecondsPerTick)
	}
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

// viewOf is the view predicted shot p meets Bodies and Echoes at this tick: its caster's (flight's View
// rule), or false when p isn't predicted or its caster has applied no frame.
func viewOf(w *cardinal.World, tuning ProjectileTuning, p component.Projectile) (float64, bool) {
	caster := w.Entity(p.Cause.Caster)
	if !p.Predicted || !caster.Has[playercomp.InputQueue]() {
		return 0, false
	}
	tick := w.TickHeight()
	view, ok := caster.Get[playercomp.InputQueue]().View(tick)
	return tuning.View(view, tick), ok
}

// seenAt is target e where the predictor's view had it, its row there from its TargetHistory, or false
// when it had none. The predictor's own Body is where it is now, with its tags now, as its client
// predicts it; so is its Echo, with its rows' tags, since its client predicts that too.
func seenAt(e cardinal.Entity, predictor cardinal.EntityID, view float64, tick uint64) (flight.Seen, bool) {
	if !e.Has[component.TargetHistory]() || !e.Has[ref.Spawn]() {
		return flight.Seen{}, false
	}
	s := flight.Seen{ID: uint32(e.ID()), Serial: e.Get[ref.Spawn]().Serial}
	if e.ID() == predictor {
		s.Row = rowOf(e, tick)
		s.Tags = tagsAt(e, tick)
		return s, true
	}
	history := e.Get[component.TargetHistory]()
	a, b := flight.RowTicks(view)
	newer, ok := history.At(b)
	if !ok {
		return flight.Seen{}, false
	}
	older, hasOlder := history.At(a)
	s.Row, ok = flight.AtView(older, hasOlder, newer, view)
	if s.Kind == interact.Echo && s.Side.Owner == uint32(predictor) {
		s.Center = box2d.Vec2(e.Get[physics2d.Transform2D]().Position)
	}
	return s, ok
}

// reachOf is where each target can be at any view this tick: between its kept rows, or where it is
// now, as seenAt places it.
func reachOf(w *cardinal.World) *flight.Reach {
	reach := flight.NewReach()
	tick := w.TickHeight()
	for e := range w.Contains[historyRow]().Iter() {
		if e.Get[physics2d.PhysicsBody2D]().Shapes.Len() == 0 {
			continue
		}
		kept := slices.Collect(e.Get[component.TargetHistory]().Rows.Values())
		reach.Add(uint32(e.ID()), rowOf(e, tick), kept...)
	}
	return reach
}

// targetsOf says whose each target's shape in world is, with its side and its tags this tick.
func targetsOf(w *cardinal.World, world *box2d.World) flight.Targets {
	tick := w.TickHeight()
	return func(shape box2d.ShapeID) (flight.Target, bool) {
		e := w.Entity(physics.EntityOf(world, shape))
		if !e.Has[team.Team]() || !e.Has[team.Owner]() || !e.Has[effectcomp.Effects]() || !e.Has[ref.Spawn]() {
			return flight.Target{}, false
		}
		return flight.Target{
			ID:     uint32(e.ID()),
			Serial: e.Get[ref.Spawn]().Serial,
			Side:   team.SideOf(e),
			Tags:   e.Get[effectcomp.Effects]().Status(tick).Tags,
		}, true
	}
}
