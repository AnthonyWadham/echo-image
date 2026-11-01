package flight

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
)

// Row is a target as one tick's syncs show it: what it is, whose, where, its size, and the tags its
// effects give it from the next tick on, as that tick's rules read them. It is the one record of a
// target: TargetHistory keeps its rows, and the target sync sends each tick's.
type Row struct {
	Tick   uint64
	Kind   interact.Kind
	Side   interact.Side
	Center box2d.Vec2
	Radius float64
	Tags   interact.Tags
	Until  float64 // the part of the tick it lived: 1, or when it died in it
}

// RowTicks is the ticks of the rows view v is drawn from: b - 1 and b = ⌈v⌉. v is t = v - (b - 1) of the
// way through tick b, 0 < t <= 1, so a whole tick is its end. The client's Timeline.sample picks the
// same two.
func RowTicks(view float64) (older, newer uint64) {
	b := uint64(math.Ceil(view))
	return b - 1, b
}

// AtView is a target where view v showed it, from its rows at RowTicks(v), or false if it showed none.
// It is newer, its row at b, drawn partway from older, its row at b - 1: t / newer.Until of the way, and
// gone past newer.Until. With no row at b - 1 (it is new at b), it is newer as it is. This is the rule
// the client draws by (EntityViews' blend); testdata/view.json pins the two together.
func AtView(older Row, hasOlder bool, newer Row, view float64) (Row, bool) {
	t := view - (math.Ceil(view) - 1)
	if t > newer.Until {
		return Row{}, false
	}
	if !hasOlder {
		return newer, true
	}
	k := t / newer.Until
	newer.Center = box2d.Vec2{
		X: older.Center.X + float64((newer.Center.X-older.Center.X)*k),
		Y: older.Center.Y + float64((newer.Center.Y-older.Center.Y)*k),
	}
	return newer, true
}

// Seen is a target where a client's view had it: its entity and serial, and its row there.
type Seen struct {
	ID     uint32
	Serial uint64
	Row
}

// Sights returns target id (its entity) where a client's view had it, or false when it wasn't there then.
type Sights func(id uint32) (Seen, bool)

// Reach is where each target can be at any view: one Box2D tree box per target, around every spot a
// view can place it at. A shot judged at a view meets only the targets whose box its path overlaps.
type Reach struct {
	tree box2d.DynamicTree
}

// NewReach returns a Reach with no targets.
func NewReach() *Reach {
	return &Reach{tree: box2d.NewDynamicTree()}
}

// Add adds target id, which a view can place where it is now or between any of its kept rows.
func (r *Reach) Add(id uint32, now Row, kept ...Row) {
	lo, hi, radius := now.Center, now.Center, now.Radius
	for _, row := range kept {
		lo, hi, radius = box2d.Min(lo, row.Center), box2d.Max(hi, row.Center), max(radius, row.Radius)
	}
	pad := box2d.Vec2{X: radius, Y: radius}
	box := box2d.AABB{LowerBound: box2d.Sub(lo, pad), UpperBound: box2d.Add(hi, pad)}
	r.tree.CreateProxy(box, uint64(now.Kind), uint64(id))
}

// View is the view a shot a client predicts is judged at on tick: the one its client drew, held to the
// views the rows kept by then are drawn from, RewindTicks before tick up to tick - 1, the newest row (and
// 1 at least, as rows start at tick 0). An older view is judged at the oldest, so a high ping must lead
// its shots but still hits. A newer one, or one that isn't a number, is judged at the newest.
func (t Tuning) View(view float64, tick uint64) float64 {
	newest := max(float64(tick)-1, 1)
	if !(view <= newest) {
		return newest
	}
	return max(view, float64(tick)-float64(t.RewindTicks()), 1)
}

// FlyAtView is Fly for a shot a client predicts. The map is world's now, and each target is where the
// client's view drew it. One query of reach finds the targets the path can meet, however fast they
// moved since, and sights places each. A pair cast in each one's own frame gets the bits the client's
// world cast gets. Ties go as in Fly.
func (t Tuning) FlyAtView(world *box2d.World, def Shot, f Flight, side interact.Side, reach *Reach, sights Sights,
	dt float64,
) (next Flight, end End, over bool) {
	to, flown, seconds, last := f.step(dt)
	first, found := t.firstHit(world, def, f.Pos, to, f.Radius, side, nil) // the map
	pad := box2d.Vec2{X: f.Radius, Y: f.Radius}
	path := box2d.AABB{LowerBound: box2d.Sub(box2d.Min(f.Pos, to), pad), UpperBound: box2d.Add(box2d.Max(f.Pos, to), pad)}
	reach.tree.Query(path, uint64(targetKinds), func(_ int, id uint64, _ any) bool {
		if s, ok := sights(uint32(id)); ok {
			if h, ok := t.hitOn(def, f.Pos, to, f.Radius, side, s); ok && (!found || h.before(first)) {
				first, found = h, true
			}
		}
		return true
	}, nil)
	return f.advance(to, flown, seconds, dt, first, found, last)
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
	return hit{fraction: out.Fraction, rule: i, kind: s.Kind, target: s.ID, serial: s.Serial, center: s.Center, meet: m},
		true
}
