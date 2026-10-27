// Package flight is the Cardinal-free core of shots: the shot types, the rules for what a shot does on
// meeting each thing, and one tick of a shot's flight. projectile.FlightSystem flies every shot with
// it, and the client predictor (internal/predict) flies your own shots with the same code. It imports
// only box2d, interact and the standard library, so it builds for wasm.
package flight

import (
	"cmp"
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
)

// Flight is a shot in flight: where it is, its velocity in units per second, its radius, and the
// distance it flies before it expires.
type Flight struct {
	Pos, Velocity box2d.Vec2
	Radius        float64
	Remaining     float64
}

// Launch is the flight of a shot of type def that leaves from along dir, a unit vector.
func Launch(def Shot, from, dir box2d.Vec2) Flight {
	return Flight{
		Pos:       from,
		Velocity:  box2d.Vec2{X: dir.X * def.Speed, Y: dir.Y * def.Speed},
		Radius:    def.Radius,
		Remaining: def.Range,
	}
}

// Target is a Body or Echo circle a shot can meet, in world space.
type Target struct {
	ID     uint32 // its entity
	Kind   interact.Kind
	Side   interact.Side
	Tags   interact.Tags
	Center box2d.Vec2
	Radius float64
}

// Hit is what a shot ended at. It is zero when the shot flew its range, and has no Target on the map.
type Hit struct {
	Target uint32
	Meet   Meet
}

// hit is a thing on a shot's path that it doesn't pass.
type hit struct {
	fraction float64
	rule     int // the index of the rule that met it
	onMap    bool
	target   uint32
	meet     Meet
}

// Fly flies f, a shot of type def from side, one tick of dt seconds through world's map and past
// targets. It returns f after the tick, and over=true when the shot ends in it: at its range, or at
// hit, the first thing on its path that its rules don't pass. A shot launched this tick flies this
// tick, so its path starts at the shooter.
func (t Tuning) Fly(world *box2d.World, def Shot, f Flight, side interact.Side, targets []Target, dt float64,
) (next Flight, over bool, h Hit) {
	speed := box2d.Length(f.Velocity)
	seconds := dt
	// Decided before the step: a remainder after it can round to a hair above 0 and never reach it.
	last := f.Remaining <= float64(speed*seconds)
	if last {
		seconds = f.Remaining / speed // the last step stops at the range
	}
	end := box2d.Vec2{X: f.Pos.X + float64(f.Velocity.X*seconds), Y: f.Pos.Y + float64(f.Velocity.Y*seconds)}
	if first, found := t.firstHit(world, def, f.Pos, end, f.Radius, side, targets); found {
		return f, true, Hit{Target: first.target, Meet: first.meet} // Stop is the only response that ends at a thing
	}
	if last {
		return f, true, Hit{} // flew its range
	}
	f.Pos = end
	f.Remaining -= float64(speed * seconds)
	return f, false, Hit{}
}

// firstHit returns the nearest thing on the path that the shot doesn't pass. At the same point the
// earlier rule wins, then the map, then the lower target ID, so by default a Body standing on its
// Echo shields it.
func (t Tuning) firstHit(world *box2d.World, def Shot, start, end box2d.Vec2, radius float64, side interact.Side,
	targets []Target,
) (hit, bool) {
	var first hit
	found := false
	keep := func(h hit) {
		if !found || cmp.Or(cmp.Compare(h.fraction, first.fraction), cmp.Compare(h.rule, first.rule),
			compareBool(h.onMap, first.onMap), cmp.Compare(h.target, first.target)) < 0 {
			first, found = h, true
		}
	}
	// The map is neutral and has no tags or Health, so one sweep covers the map kinds it stops at.
	var stops interact.Kind
	rule := math.MaxInt
	for _, k := range []interact.Kind{interact.Wall, interact.Bounds} {
		if m, i := t.meet(def, k, interact.Neutral, 0); m.Do != Pass {
			stops, rule = stops|k, min(rule, i)
		}
	}
	if stops != 0 {
		if f, ok := sweepMap(world, start, end, radius, stops); ok {
			keep(hit{fraction: f, rule: rule, onMap: true, meet: Meet{Do: Stop}})
		}
	}
	for _, target := range targets {
		m, i := t.meet(def, target.Kind, side.Relation(target.Side), target.Tags)
		if m.Do == Pass {
			continue
		}
		if f, ok := sweep(start, end, target.Center, target.Radius+radius); ok {
			keep(hit{fraction: f, rule: i, target: target.ID, meet: m})
		}
	}
	return first, found
}

// compareBool orders true before false.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

// sweepMap returns the first fraction of the path start→end at which a circle of radius meets a map
// shape of a kind in mask, as physics2d's CircleSweep finds it: Box2D's shape cast, skipping sensors.
func sweepMap(world *box2d.World, start, end box2d.Vec2, radius float64, mask interact.Kind) (float64, bool) {
	if radius <= 0 || start == end {
		return 0, false
	}
	var proxy box2d.ShapeProxy
	proxy.Points[0] = start
	proxy.Count = 1
	proxy.Radius = radius
	c := &mapHit{world: world}
	filter := box2d.QueryFilter{CategoryBits: uint64(interact.Shot), MaskBits: uint64(mask)}
	world.CastShape(&proxy, box2d.Vec2{X: end.X - start.X, Y: end.Y - start.Y}, filter, closest, c)
	return c.fraction, c.hit
}

// mapHit is the nearest map hit a shape cast has found so far.
type mapHit struct {
	world    *box2d.World
	hit      bool
	fraction float64
}

// closest keeps each hit that isn't a sensor and returns its fraction, so Box2D only looks closer.
func closest(shape box2d.ShapeID, _, _ box2d.Vec2, fraction float64, ctx any) float64 {
	c, ok := ctx.(*mapHit)
	if !ok {
		return 0
	}
	if c.world.IsShapeSensor(shape) {
		return -1 // skip sensors
	}
	c.hit, c.fraction = true, fraction
	return fraction
}

// sweep returns the first fraction of the path start→end at which a point comes within r of c. A path
// that starts within r hits at 0. Each product is rounded, so no target fuses it into an FMA.
func sweep(start, end, c box2d.Vec2, r float64) (float64, bool) {
	dx, dy := end.X-start.X, end.Y-start.Y
	fx, fy := start.X-c.X, start.Y-c.Y
	cc := float64(fx*fx) + float64(fy*fy) - float64(r*r)
	if cc <= 0 {
		return 0, true
	}
	b := float64(fx*dx) + float64(fy*dy)
	if b >= 0 {
		return 0, false // not moving toward c
	}
	a := float64(dx*dx) + float64(dy*dy)
	disc := float64(b*b) - float64(a*cc)
	if disc < 0 {
		return 0, false
	}
	t := (-b - math.Sqrt(disc)) / a
	return t, t <= 1
}
