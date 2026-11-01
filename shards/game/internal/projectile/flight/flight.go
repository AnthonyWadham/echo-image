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

// Target is whose a target's shape is, as a shot's rules see it.
type Target struct {
	ID     uint32 // its entity
	Serial uint64 // its entity's ref.Spawn serial: ID may name a newer entity next tick
	Side   interact.Side
	Tags   interact.Tags
}

// Targets returns the Target a target's shape in the world belongs to, or false for none. Nil means
// shots meet no targets, as in a client predictor without the syncs of its view.
type Targets func(shape box2d.ShapeID) (Target, bool)

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
// there (0 to 1), and what it met.
type End struct {
	At    box2d.Vec2
	Until float64
	Hit   Hit
}

// hit is a thing on a shot's path that it doesn't pass.
type hit struct {
	fraction float64
	rule     int // the index of the rule that met it
	kind     interact.Kind
	target   uint32
	serial   uint64
	center   box2d.Vec2 // a target's, where the cast met it
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

// Fly flies f, a shot of type def from side, one tick of dt seconds through world, where targets says
// whose each target's shape is. It returns f after the tick, or over=true and how it ended in it: at
// its range, or at the first thing on its path that its rules don't pass. A shot launched this tick flies
// this tick, so its path starts at the shooter.
func (t Tuning) Fly(world *box2d.World, def Shot, f Flight, side interact.Side, targets Targets, dt float64,
) (next Flight, end End, over bool) {
	to, flown, seconds, last := f.step(dt)
	first, found := t.firstHit(world, def, f.Pos, to, f.Radius, side, targets)
	return f.advance(to, flown, seconds, dt, first, found, last)
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

// firstHit returns the nearest thing on the path that the shot doesn't pass, from one Box2D shape cast
// over the map and the targets. At the same point the earlier rule wins, then the map, then the lower
// target ID, so by default a Body standing on its Echo shields it.
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
		mask |= targetKinds
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
	category := interact.Kind(c.world.ShapeFilter(shape).CategoryBits)
	h := hit{fraction: fraction, rule: c.mapRule, kind: category & interact.Map, meet: Meet{Do: Stop}}
	if kind := category & targetKinds; kind != 0 {
		target, ok := c.targets(shape)
		if !ok {
			return -1
		}
		m, i := c.tuning.meet(c.def, kind, c.side.Relation(target.Side), target.Tags)
		if m.Do == Pass {
			return -1
		}
		center := c.world.BodyPosition(c.world.ShapeBody(shape)) // its circle sits on its body's origin
		h = hit{fraction: fraction, rule: i, kind: kind, target: target.ID, serial: target.Serial, center: center, meet: m}
	}
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
