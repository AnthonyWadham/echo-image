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

// Target is whose a Body or Echo shape is, as a shot's rules see it.
type Target struct {
	ID   uint32 // its entity
	Side interact.Side
	Tags interact.Tags
}

// Targets returns the Target a Body or Echo shape in the world belongs to, or false for none. Nil means
// the world has no Bodies or Echoes for shots to meet, as in the client predictor's.
type Targets func(shape box2d.ShapeID) (Target, bool)

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

// Fly flies f, a shot of type def from side, one tick of dt seconds through world, where targets says
// whose each Body and Echo shape is. It returns f after the tick, and over=true when the shot ends in
// it: at its range, or at hit, the first thing on its path that its rules don't pass. A shot launched
// this tick flies this tick, so its path starts at the shooter.
func (t Tuning) Fly(world *box2d.World, def Shot, f Flight, side interact.Side, targets Targets, dt float64,
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

// firstHit returns the nearest thing on the path that the shot doesn't pass, from one Box2D shape cast
// over the map and the Bodies and Echoes. At the same point the earlier rule wins, then the map, then
// the lower target ID, so by default a Body standing on its Echo shields it.
func (t Tuning) firstHit(world *box2d.World, def Shot, start, end box2d.Vec2, radius float64, side interact.Side,
	targets Targets,
) (hit, bool) {
	if radius <= 0 || start == end {
		return hit{}, false
	}
	c := &cast{tuning: t, def: def, world: world, side: side, targets: targets, mapRule: math.MaxInt}
	// The map is neutral and has no tags or Health, so one rule covers the map kinds it stops at.
	var mask interact.Kind
	for _, k := range []interact.Kind{interact.Wall, interact.Bounds} {
		if m, i := t.meet(def, k, interact.Neutral, 0); m.Do != Pass {
			mask, c.mapRule = mask|k, min(c.mapRule, i)
		}
	}
	if targets != nil {
		mask |= interact.Body | interact.Echo
	}
	if mask == 0 {
		return hit{}, false
	}
	var proxy box2d.ShapeProxy
	proxy.Points[0] = start
	proxy.Count = 1
	proxy.Radius = radius
	filter := box2d.QueryFilter{CategoryBits: uint64(interact.Shot), MaskBits: uint64(mask)}
	world.CastShape(&proxy, box2d.Vec2{X: end.X - start.X, Y: end.Y - start.Y}, filter, meetShape, c)
	return c.first, c.found
}

// cast is one shot's shape cast: what judging a shape needs, and the nearest hit so far.
type cast struct {
	tuning  Tuning
	def     Shot
	world   *box2d.World
	side    interact.Side
	targets Targets
	mapRule int // the earliest rule that stops the shot at the map
	first   hit
	found   bool
}

// meetShape judges each shape the cast finds by the shot's rules and keeps the nearest it doesn't
// pass. It returns 1 to keep looking, so a shape at the same point can still win the tie.
func meetShape(shape box2d.ShapeID, _, _ box2d.Vec2, fraction float64, ctx any) float64 {
	c, ok := ctx.(*cast)
	if !ok {
		return 0
	}
	if c.world.IsShapeSensor(shape) {
		return -1 // skip sensors
	}
	h := hit{fraction: fraction, rule: c.mapRule, onMap: true, meet: Meet{Do: Stop}}
	if kind := interact.Kind(c.world.ShapeFilter(shape).CategoryBits) & (interact.Body | interact.Echo); kind != 0 {
		target, ok := c.targets(shape)
		if !ok {
			return -1
		}
		m, i := c.tuning.meet(c.def, kind, c.side.Relation(target.Side), target.Tags)
		if m.Do == Pass {
			return -1
		}
		h = hit{fraction: fraction, rule: i, target: target.ID, meet: m}
	}
	if !c.found || cmp.Or(cmp.Compare(h.fraction, c.first.fraction), cmp.Compare(h.rule, c.first.rule),
		compareBool(h.onMap, c.first.onMap), cmp.Compare(h.target, c.first.target)) < 0 {
		c.first, c.found = h, true
	}
	return 1
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
