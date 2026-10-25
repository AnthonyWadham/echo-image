package projectile

import (
	"cmp"
	"math"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/projectile/component"
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

// target is one Body or Echo circle, in world space.
type target struct {
	id     cardinal.EntityID
	kind   interact.Kind
	side   interact.Side
	tags   interact.Tags
	center physics2d.Vec2
	radius float64
}

// hit is a thing on a shot's path that it doesn't pass.
type hit struct {
	fraction float64
	rule     int // the index of the rule that met it
	target   cardinal.EntityID
	meet     Meet
}

// FlightSystem (Update, after LaunchSystem, before combat.DamageResolveSystem) moves every shot one
// tick along its path and resolves the first thing on it that the shot doesn't pass. Rules decide what
// a shot does on meeting each thing: the shot type's own, then the defaults (GDD §7). It passes the
// thing or stops there, and a payload rule gives the thing its damage as a DamageIntent. A shot
// launched this tick moves this tick, so its path starts at the shooter. O(P·T·R) for P shots, T
// Body/Echo circles and R rules, plus one CircleSweep per shot.
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
	// Resolved after the search finished, since a resolved shot is destroyed.
	for _, shot := range shots {
		s.fly(w, tuning, shot, targets)
	}
}

func (s *FlightSystem) fly(w *cardinal.World, tuning ProjectileTuning, shot cardinal.Entity, targets []target) {
	p := shot.Get[component.Projectile]()
	def, ok := tuning.Shots[p.Type]
	if !ok {
		w.Logger().Error().Str("type", p.Type).Msg("removed a shot of unknown type")
		shot.Destroy()
		return
	}
	start := shot.Get[physics2d.Transform2D]().Position
	speed := math.Hypot(p.Velocity.X, p.Velocity.Y)
	seconds := math.Min(physics.SecondsPerTick, p.Remaining/speed) // the last step stops at the range
	end := physics2d.Vec2{X: start.X + p.Velocity.X*seconds, Y: start.Y + p.Velocity.Y*seconds}

	first, found := s.firstHit(tuning, def, start, end, p.Radius, team.SideOf(shot), targets)
	if !found {
		p.Remaining -= speed * seconds
		if p.Remaining <= 0 {
			shot.Destroy() // flew its range
			return
		}
		shot.Set(p)
		shot.Set(physics2d.Transform2D{Position: end})
		return
	}
	if first.meet.Payload {
		w.EmitSystemEvent(combatcomp.DamageIntent{
			Target: first.target, Credit: shot.Get[team.Owner]().Body, Cause: p.Cause, Amount: p.Damage,
		})
	}
	shot.Destroy() // Stop is the only response that ends at a thing
}

// firstHit returns the nearest thing on the path that the shot doesn't pass. At the same point the
// earlier rule wins, then the lower entity ID, so by default a Body standing on its Echo shields it.
func (s *FlightSystem) firstHit(tuning ProjectileTuning, def Shot, start, end physics2d.Vec2, radius float64,
	side interact.Side, targets []target,
) (hit, bool) {
	var first hit
	found := false
	keep := func(h hit) {
		if !found || cmp.Or(cmp.Compare(h.fraction, first.fraction), cmp.Compare(h.rule, first.rule),
			cmp.Compare(h.target, first.target)) < 0 {
			first, found = h, true
		}
	}
	// The map is neutral and has no tags or Health, so one sweep covers the map kinds it stops at.
	var stops interact.Kind
	rule := math.MaxInt
	for _, k := range []interact.Kind{interact.Wall, interact.Bounds} {
		if m, i := tuning.meet(def, k, interact.Neutral, 0); m.Do != Pass {
			stops, rule = stops|k, min(rule, i)
		}
	}
	if stops != 0 {
		wall := s.Physics.CircleSweep(physics2d.CircleSweepRequest{
			Start:  start,
			End:    end,
			Radius: radius,
			Filter: &physics2d.Filter{CategoryBits: uint64(interact.Shot), MaskBits: uint64(stops)},
		})
		if wall.Hit {
			keep(hit{fraction: wall.Fraction, rule: rule, target: wall.Entity, meet: Meet{Do: Stop}})
		}
	}
	for _, t := range targets {
		m, i := tuning.meet(def, t.kind, side.Relation(t.side), t.tags)
		if m.Do == Pass {
			continue
		}
		if f, ok := sweep(start, end, t.center, t.radius+radius); ok {
			keep(hit{fraction: f, rule: i, target: t.id, meet: m})
		}
	}
	return first, found
}

// collectTargets lists every Body and Echo circle once per tick, so each shot tests plain values.
func collectTargets(w *cardinal.World) []target {
	tick := w.TickHeight()
	var targets []target
	for e := range w.Contains[targetRow]().Iter() {
		pose := e.Get[physics2d.Transform2D]()
		side := team.SideOf(e)
		tags := e.Get[effectcomp.Effects]().Status(tick).Tags
		for shape := range e.Get[physics2d.PhysicsBody2D]().Shapes.Values() {
			kind := interact.Kind(shape.CategoryBits) & (interact.Body | interact.Echo)
			if kind == 0 || shape.ShapeType != physics2d.ShapeTypeCircle {
				continue
			}
			targets = append(targets, target{
				id:     e.ID(),
				kind:   kind,
				side:   side,
				tags:   tags,
				center: physics.WorldPoint(pose, shape.LocalOffset),
				radius: shape.Radius,
			})
		}
	}
	return targets
}

// sweep returns the first fraction of the path start→end at which a point comes within r of c. A path
// that starts within r hits at 0.
func sweep(start, end, c physics2d.Vec2, r float64) (float64, bool) {
	dx, dy := end.X-start.X, end.Y-start.Y
	fx, fy := start.X-c.X, start.Y-c.Y
	cc := fx*fx + fy*fy - r*r
	if cc <= 0 {
		return 0, true
	}
	b := fx*dx + fy*dy
	if b >= 0 {
		return 0, false // not moving toward c
	}
	a := dx*dx + dy*dy
	disc := b*b - a*cc
	if disc < 0 {
		return 0, false
	}
	t := (-b - math.Sqrt(disc)) / a
	return t, t <= 1
}
