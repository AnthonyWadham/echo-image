package flight

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
)

// Row is a Body or Echo as one tick's syncs show it: where it is, its size, and the tags its effects
// give it from the next tick on, as that tick's rules read them.
type Row struct {
	Tick   uint64
	Center box2d.Vec2
	Radius float64
	Tags   interact.Tags
	Until  float64 // the part of the tick it lived: 1, or when it died in it
}

// AtView is a target where a view showed it, from its rows around the view, or false if it showed none.
// At view v, t of the way from tick a = floor(v) to a + 1, it is newer, its row at a + 1, drawn partway
// from older, its row at a: t / newer.Until of the way, and gone past newer.Until. With no row at a (it
// is new at a + 1), it is newer as it is. This is the rule the client draws by (EntityViews' blend).
func AtView(older Row, hasOlder bool, newer Row, view float64) (Row, bool) {
	t := view - math.Floor(view)
	if t > newer.Until {
		return Row{}, false
	}
	if !hasOlder {
		return newer, true
	}
	k := 1.0
	if newer.Until > 0 {
		k = t / newer.Until
	}
	newer.Center = box2d.Vec2{
		X: older.Center.X + float64((newer.Center.X-older.Center.X)*k),
		Y: older.Center.Y + float64((newer.Center.Y-older.Center.Y)*k),
	}
	return newer, true
}

// Seen is a Body or Echo as a hit claim's view had it: whose it is, its kind, and its circle then.
type Seen struct {
	Target
	Kind   interact.Kind // Body or Echo
	Center box2d.Vec2
	Radius float64
}

// Sights returns the Body or Echo a shape in the world belongs to, as a claim's view had it, or false
// when it wasn't there then.
type Sights func(shape box2d.ShapeID) (Seen, bool)

// Confirm checks a hit claim: that f, a shot of type def from side, stops at claimed first on its path
// this tick, as Fly would with every Body and Echo where the claim's view had them. The map is world's
// now. One Box2D overlap of the path, grown by how far a Body or Echo moves in the view's age (ticks),
// finds the ones that could come first, and sights places each. It returns how the shot ends on
// claimed, or false when the path misses it, its rules pass it, or the map or another target comes
// first. Ties go as in Fly.
func (t Tuning) Confirm(world *box2d.World, def Shot, f Flight, side interact.Side, claimed Seen, age float64,
	sights Sights, dt float64,
) (End, bool) {
	to, _, seconds, _ := f.step(dt)
	h, ok := t.hitOn(def, f.Pos, to, f.Radius, side, claimed)
	if !ok {
		return End{}, false
	}
	if first, found := t.firstHit(world, def, f.Pos, to, f.Radius, side, nil); found && first.before(h) {
		return End{}, false
	}
	pad := float64(float64(t.MaxTargetSpeed*age) * dt)
	proxy := box2d.MakeProxy([]box2d.Vec2{f.Pos, to}, 2, f.Radius+pad)
	filter := box2d.QueryFilter{CategoryBits: uint64(interact.Shot), MaskBits: uint64(interact.Body | interact.Echo)}
	first := true
	world.OverlapShape(&proxy, filter, func(shape box2d.ShapeID, _ any) bool {
		if world.IsShapeSensor(shape) {
			return true
		}
		if s, ok := sights(shape); ok && s.ID != claimed.ID {
			if o, ok := t.hitOn(def, f.Pos, to, f.Radius, side, s); ok && o.before(h) {
				first = false
			}
		}
		return first
	}, nil)
	if !first {
		return End{}, false
	}
	return f.endAt(to, seconds/dt, h), true
}

// hitOn is where the path from start to end meets s, from one Box2D pair cast in s's own frame. That is
// how a world cast meets each circle, so it gives Fly's bits. False when it misses s or the rules pass
// it.
func (t Tuning) hitOn(def Shot, start, end box2d.Vec2, radius float64, side interact.Side, s Seen) (hit, bool) {
	if radius <= 0 || start == end {
		return hit{}, false
	}
	m, i := t.meet(def, s.Kind, side.Relation(s.Side), s.Tags)
	if m.Do == Pass {
		return hit{}, false
	}
	frame := box2d.Transform{P: s.Center, Q: box2d.RotIdentity}
	out := box2d.ShapeCastCircle(&box2d.Circle{Radius: s.Radius}, &box2d.ShapeCastInput{
		Proxy:       box2d.MakeProxy([]box2d.Vec2{box2d.InvTransformPoint(frame, start)}, 1, radius),
		Translation: box2d.InvRotateVector(frame.Q, box2d.Vec2{X: end.X - start.X, Y: end.Y - start.Y}),
		MaxFraction: 1,
	})
	if !out.Hit {
		return hit{}, false
	}
	return hit{fraction: out.Fraction, rule: i, kind: s.Kind, target: s.ID, meet: m}, true
}

// ValidView reports whether a hit claim at tick may use view: a tick at most RewindTicks before it, whose
// rows were saved by the tick before. NaN and the infinities fail.
func (t Tuning) ValidView(view float64, tick uint64) bool {
	return view >= 0 && float64(tick)-view <= float64(t.RewindTicks()) && math.Floor(view)+1 < float64(tick)
}
