package projectile

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/team"
)

// targetRow is an entity whose Body or Echo circles a shot can hit.
type targetRow struct {
	Team      team.Team
	Transform physics2d.Transform2D
	Body      physics2d.PhysicsBody2D
}

// target is one Body or Echo circle, in world space.
type target struct {
	id     cardinal.EntityID
	team   uint8
	center physics2d.Vec2
	radius float64
	echo   bool
}

// hit is the first thing on a shot's path; echo is false for a wall or a Body.
type hit struct {
	fraction float64
	echo     bool
	target   cardinal.EntityID
}

// FlightSystem (Update, after LaunchSystem, before combat.DamageResolveSystem) moves every shot one
// tick along its path and resolves the first hit: a wall or an enemy Body destroys it, and an enemy
// Echo takes its damage as a DamageIntent. A shot launched this tick moves this tick, so its path
// starts at the shooter. O(P·T) for P shots and T Body/Echo circles, plus one CircleSweep per shot.
type FlightSystem struct {
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
	targets := collectTargets(w)
	// Resolved after the search finished, since a resolved shot is destroyed.
	for _, shot := range shots {
		s.fly(w, shot, targets)
	}
}

func (s *FlightSystem) fly(w *cardinal.World, shot cardinal.Entity, targets []target) {
	p := shot.Get[component.Projectile]()
	start := shot.Get[physics2d.Transform2D]().Position
	speed := math.Hypot(p.Velocity.X, p.Velocity.Y)
	seconds := math.Min(physics.SecondsPerTick, p.Remaining/speed) // the last step stops at the range
	end := physics2d.Vec2{X: start.X + p.Velocity.X*seconds, Y: start.Y + p.Velocity.Y*seconds}

	first, found := s.firstHit(start, end, p.Radius, shot.Get[team.Team]().ID, targets)
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
	if first.echo {
		w.EmitSystemEvent(combatcomp.DamageIntent{Target: first.target, Source: p.Source, Amount: p.Damage})
	}
	shot.Destroy()
}

// firstHit returns the nearest wall, enemy Body or enemy Echo on the path. A Body beats an Echo at the
// same fraction, so a Body standing on its Echo shields it.
func (s *FlightSystem) firstHit(start, end physics2d.Vec2, radius float64, shotTeam uint8, targets []target) (hit, bool) {
	var first hit
	found := false
	wall := s.Physics.CircleSweep(physics2d.CircleSweepRequest{
		Start:  start,
		End:    end,
		Radius: radius,
		Filter: &physics2d.Filter{CategoryBits: physics.CategoryProjectile, MaskBits: physics.CategoryEnvironment},
	})
	if wall.Hit {
		first, found = hit{fraction: wall.Fraction}, true
	}
	for _, t := range targets {
		if t.team == shotTeam {
			continue // shots pass through their own team
		}
		f, ok := sweep(start, end, t.center, t.radius+radius)
		if !ok {
			continue
		}
		if !found || f < first.fraction || (f == first.fraction && !t.echo) {
			first, found = hit{fraction: f, echo: t.echo, target: t.id}, true
		}
	}
	return first, found
}

// collectTargets lists every Body and Echo circle once per tick, so each shot tests plain values.
func collectTargets(w *cardinal.World) []target {
	var targets []target
	for e := range w.Contains[targetRow]().Iter() {
		pose := e.Get[physics2d.Transform2D]()
		teamID := e.Get[team.Team]().ID
		for shape := range e.Get[physics2d.PhysicsBody2D]().Shapes.Values() {
			kind := shape.CategoryBits & (physics.CategoryBody | physics.CategoryEcho)
			if kind == 0 || shape.ShapeType != physics2d.ShapeTypeCircle {
				continue
			}
			targets = append(targets, target{
				id:     e.ID(),
				team:   teamID,
				center: worldPoint(pose, shape.LocalOffset),
				radius: shape.Radius,
				echo:   kind&physics.CategoryEcho != 0,
			})
		}
	}
	return targets
}

func worldPoint(pose physics2d.Transform2D, local physics2d.Vec2) physics2d.Vec2 {
	c, s := math.Cos(pose.Rotation), math.Sin(pose.Rotation)
	return physics2d.Vec2{X: pose.Position.X + c*local.X - s*local.Y, Y: pose.Position.Y + s*local.X + c*local.Y}
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
