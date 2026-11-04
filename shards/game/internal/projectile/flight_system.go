package projectile

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	playercomp "echo_image/shards/game/internal/player/component"
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
// A shot meets the map now, and each Body and Echo as it moves across the views its tick is drawn over
// (past.Across), from their TargetHistory: a predicted shot across its caster's frame's view and the
// next, as that client drew it, and any other (an Echo's) across this tick, as every screen draws it.
// That is what the client's predictor flies it against, so a hit on screen is a hit on the server. One
// Box2D cast and tree query per shot finds what it meets, so the cost is O(P·(log N + K·R)) for P shots,
// N shapes, K found and R rules, plus O(T·W) a tick for the tree of T targets keeping W rows.
type FlightSystem struct {
	Tuning  ProjectileTuning
	Window  past.Window // how far back a predicted shot's view may be
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
	world, reach := s.Physics.Engine(), reachOf(w)
	// Resolved after the search finished, since a resolved shot is destroyed.
	for _, shot := range shots {
		fly(w, world, s.Tuning, s.Window, shot, reach)
	}
}

func fly(w *cardinal.World, world *box2d.World, tuning ProjectileTuning, window past.Window, shot cardinal.Entity,
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
	tick := w.TickHeight()
	view, predicted := viewOf(w, window, p)
	if !predicted {
		view = max(float64(tick)-1, 0) // this tick's views
	}
	sights := func(key uint64) (flight.Seen, bool) {
		return seenAt(w.Entity(cardinal.EntityID(key)), view, tick, p.Cause.Caster, predicted)
	}
	next, end, over := tuning.Fly(world, def, f, team.SideOf(shot), reach, sights, physics.SecondsPerTick)
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

// viewOf is the view predicted shot p meets Bodies and Echoes at this tick: its caster's, held by the
// window (past.Window.Clamp), or false when p isn't predicted or its caster has applied no frame.
func viewOf(w *cardinal.World, window past.Window, p component.Projectile) (float64, bool) {
	caster := w.Entity(p.Cause.Caster)
	if !p.Predicted || !caster.Has[playercomp.InputQueue]() {
		return 0, false
	}
	tick := w.TickHeight()
	view, ok := caster.Get[playercomp.InputQueue]().View(tick)
	return window.Clamp(view, tick), ok
}

// seenAt is target e across the views a shot's tick at view is drawn over (past.Across), from its
// TargetHistory and its row at tick: how it is now, with its tags now. For a shot its caster's client
// predicts, that caster's own Body and Echo move as its client draws them, from their rows a tick ago to
// where they are now, its Body all the tick with its tags now. False when it shows nowhere.
func seenAt(e cardinal.Entity, view float64, tick uint64, caster cardinal.EntityID, predicted bool) (flight.Seen, bool) {
	if !e.Has[component.TargetHistory]() || !e.Has[ref.Spawn]() {
		return flight.Seen{}, false
	}
	history, now := e.Get[component.TargetHistory](), rowOf(e, tick)
	now.Tags = tagsAt(e, tick)
	s := flight.Seen{ID: uint32(e.ID()), Serial: e.Get[ref.Spawn]().Serial}
	if predicted && e.ID() == caster {
		s.Spans[0] = past.Span{Row: now, End: 1}
	} else {
		s.Spans = past.Across(view, func(b uint64) (past.Row, bool) {
			if b == tick {
				return now, true
			}
			return history.At(b)
		})
	}
	if predicted && (e.ID() == caster || now.Kind == interact.Echo && now.Side.Owner == uint32(caster)) {
		prev, ok := history.At(tick - 1)
		if !ok {
			prev = now
		}
		s.Move(prev.Center, now.Center)
	}
	return s, s.Shown()
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
		reach.Add(uint64(e.ID()), rowOf(e, tick), kept...)
	}
	return reach
}
