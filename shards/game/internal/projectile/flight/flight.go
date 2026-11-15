// Package flight is the Cardinal-free core of shots: the shot types, the rules for what a shot does on
// meeting each thing, and one tick of a shot's flight. projectile.FlightSystem flies every shot with
// it, and the client predictor (internal/predict) flies your own shots with the same code. It imports
// only box2d, interact, history/past and the standard library, so it builds for wasm.
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

// Hit is what a shot ended at: its kind, and for a target its entity, serial and where the shot's
// center was from the target's (Offset), so each screen can end the shot on the target where it draws
// it. It is zero when the shot flew its range.
type Hit struct {
	Kind   interact.Kind
	Target uint32
	Serial uint64
	Offset box2d.Vec2
	Meet   Meet
}

// OnTarget reports whether the shot ended on a target, not the map or its range.
func (h Hit) OnTarget() bool { return h.Kind&^interact.Map != 0 }

// End is how a shot's flight ended in a tick: where its center was, the part of the tick it flew to get
// there (0 to 1), what it met, and the view its tick was judged at.
type End struct {
	At    box2d.Vec2
	Until float64
	Hit   Hit
	View  float64
}

// hit is a thing on a shot's path that it doesn't pass.
type hit struct {
	fraction float64
	rule     int // the index of the rule that met it
	kind     interact.Kind
	target   uint32
	serial   uint64
	center   box2d.Vec2 // a target's, when the cast met it
	meet     Meet
}

// onMap reports whether h is on the map, not a target.
func (h hit) onMap() bool { return h.kind&interact.Map != 0 }

// before reports whether h comes before o on a shot's path: the nearer first, then the earlier rule,
// then the map, then the lower target ID.
func (h hit) before(o hit) bool {
	return cmp.Or(cmp.Compare(h.fraction, o.fraction), cmp.Compare(h.rule, o.rule),
		compareBool(h.onMap(), o.onMap()), cmp.Compare(h.target, o.target)) < 0
}

// result is h for a shot that ended at at.
func (h hit) result(at box2d.Vec2) Hit {
	r := Hit{Kind: h.kind, Target: h.target, Serial: h.serial, Meet: h.meet}
	if !h.onMap() {
		r.Offset = box2d.Vec2{X: at.X - h.center.X, Y: at.Y - h.center.Y}
	}
	return r
}

// advance ends f at first when found, or at its range when last; otherwise it is f flown to to.
func (f Flight) advance(to box2d.Vec2, flown, seconds, dt float64, first hit, found, last bool) (Flight, End, bool) {
	if found {
		return f, f.endAt(to, seconds/dt, first), true // Stop is the only response that ends at a thing
	}
	if last {
		return f, End{At: to, Until: seconds / dt}, true // flew its range
	}
	f.Pos = to
	f.Remaining -= flown
	return f, End{}, false
}

// step is where f's flight this tick goes, how far and how many seconds it flies, and last=true when
// that reaches its range.
func (f Flight) step(dt float64) (to box2d.Vec2, flown, seconds float64, last bool) {
	speed := box2d.Length(f.Velocity)
	seconds = dt
	// Decided before the step: a remainder after it can round to a hair above 0 and never reach it.
	last = f.Remaining <= float64(speed*seconds)
	if last {
		seconds = f.Remaining / speed // the last step stops at the range
	}
	to = box2d.Vec2{X: f.Pos.X + float64(f.Velocity.X*seconds), Y: f.Pos.Y + float64(f.Velocity.Y*seconds)}
	return to, float64(speed * seconds), seconds, last
}

// endAt is how f, flying to to over part of the tick, ends at h: where the cast touched it.
func (f Flight) endAt(to box2d.Vec2, part float64, h hit) End {
	at := box2d.Vec2{X: f.Pos.X + float64((to.X-f.Pos.X)*h.fraction), Y: f.Pos.Y + float64((to.Y-f.Pos.Y)*h.fraction)}
	return End{At: at, Until: h.fraction * part, Hit: h.result(at)}
}

// firstHit returns the nearest part of the map on the path that the shot doesn't pass, from one Box2D
// shape cast. At the same point as a target, the earlier rule wins, then the map, then the lower target
// ID, so by default a Body standing on its Echo shields it.
func (t Tuning) firstHit(world *box2d.World, def Shot, start, end box2d.Vec2, radius float64) (hit, bool) {
	if radius <= 0 || start == end {
		return hit{}, false
	}
	c := &cast{world: world, mapRule: math.MaxInt}
	// The map is neutral and has no tags or Health, so one rule covers the map kinds it stops at.
	var mask interact.Kind
	for _, k := range []interact.Kind{interact.Wall, interact.Bounds} {
		if m, i := t.meet(def, k, interact.Neutral, 0); m.Do != Pass {
			mask, c.mapRule = mask|k, min(c.mapRule, i)
		}
	}
	if mask == 0 {
		return hit{}, false
	}
	var proxy box2d.ShapeProxy
	proxy.Points[0] = start
	proxy.Count = 1
	proxy.Radius = radius
	// A shot that starts inside a wall meets its fill: a chain outline stops it only from outside.
	filter := box2d.QueryFilter{CategoryBits: uint64(interact.Shot), MaskBits: uint64(interact.Solid(mask))}
	world.CastShape(&proxy, box2d.Vec2{X: end.X - start.X, Y: end.Y - start.Y}, filter, meetShape, c)
	return c.first, c.found
}

// cast is one shot's shape cast over the map: the rule that stops it there, and the nearest hit so far.
type cast struct {
	world   *box2d.World
	mapRule int // the earliest rule that stops the shot at the map
	first   hit
	found   bool
}

// meetShape keeps the nearest part of the map the cast finds. It returns 1 to keep looking, so a
// shape at the same point can still win the tie.
func meetShape(shape box2d.ShapeID, _, _ box2d.Vec2, fraction float64, ctx any) float64 {
	c, ok := ctx.(*cast)
	if !ok {
		return 0
	}
	if c.world.IsShapeSensor(shape) {
		return -1 // skip sensors
	}
	kind := interact.Kind(c.world.ShapeFilter(shape).CategoryBits) & interact.Map
	if kind == interact.Fill {
		kind = interact.Wall // a wall's fill is the wall
	}
	h := hit{fraction: fraction, rule: c.mapRule, kind: kind, meet: Meet{Do: Stop}}
	if !c.found || h.before(c.first) {
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
