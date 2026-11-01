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
	newer.Center = along(older, hasOlder, newer, t)
	return newer, true
}

// along is where a target drawn from its rows older to newer is t of the way through newer's tick, t at
// most newer.Until: partway from older's center at its own pace, or at newer's with no older row.
func along(older Row, hasOlder bool, newer Row, t float64) box2d.Vec2 {
	if !hasOlder {
		return newer.Center
	}
	return lerp(older.Center, newer.Center, t/newer.Until)
}

// lerp is k of the way from a to b.
func lerp(a, b box2d.Vec2, k float64) box2d.Vec2 {
	return box2d.Vec2{X: a.X + float64((b.X-a.X)*k), Y: a.Y + float64((b.Y-a.Y)*k)}
}

// Span is part of a shot's tick, Start to End (0 to 1), in which a target shows as Row, moving from
// Row.Center to To at its own pace. One with End at most Start is empty.
type Span struct {
	Row
	Start, End float64
	To         box2d.Vec2
}

// Seen is a target as a shot's tick meets it: its entity and serial, and the spans of the tick it shows
// in, one per pair of rows the tick is drawn from.
type Seen struct {
	ID     uint32
	Serial uint64
	Spans  [2]Span
}

// Shown reports whether s shows in any part of the tick.
func (s Seen) Shown() bool {
	return s.Spans[0].End > s.Spans[0].Start || s.Spans[1].End > s.Spans[1].Start
}

// Move puts s on the line from from to to over the tick, in the spans it shows in: as a client draws
// its own Body and Echo, which it predicts.
func (s *Seen) Move(from, to box2d.Vec2) {
	for i := range s.Spans {
		sp := &s.Spans[i]
		sp.Center, sp.To = lerp(from, to, sp.Start), lerp(from, to, sp.End)
	}
}

// Across is a target as views v to v + 1 show it, from row, its row at each tick: those views are what
// a shot's tick judged at view v is drawn across, since a frame's shots fly a tick while the view moves
// a tick. It has a span for each pair of rows those views are drawn from that shows it, from where
// AtView places it as the span starts to where as it ends.
func Across(v float64, row func(tick uint64) (Row, bool)) [2]Span {
	var spans [2]Span
	first := uint64(math.Floor(v)) + 1 // the newer row of the first pair
	for i := range spans {
		b := first + uint64(i)
		newer, ok := row(b)
		if !ok {
			continue
		}
		older, hasOlder := row(b - 1)
		d := float64(b-1) - v // when in the shot's tick b's views begin
		start, end := max(d, 0), min(d+newer.Until, 1)
		if !(end > start) {
			continue
		}
		spans[i] = Span{Row: newer, Start: start, End: end, To: along(older, hasOlder, newer, end-d)}
		spans[i].Center = along(older, hasOlder, newer, start-d)
	}
	return spans
}

// Sights returns the target reach knows by key as a shot's tick meets it, or false when it shows
// nowhere then.
type Sights func(key uint64) (Seen, bool)

// Reach is where each target can be at any view: one Box2D tree box per target, around every spot a
// view can place it at. A shot judged at a view meets only the targets whose box its path overlaps.
type Reach struct {
	tree box2d.DynamicTree
}

// NewReach returns a Reach with no targets.
func NewReach() *Reach {
	return &Reach{tree: box2d.NewDynamicTree()}
}

// Add adds the target sights knows by key, which a view can place where it is now or between any of its
// kept rows.
func (r *Reach) Add(key uint64, now Row, kept ...Row) {
	lo, hi, radius := now.Center, now.Center, now.Radius
	for _, row := range kept {
		lo, hi, radius = box2d.Min(lo, row.Center), box2d.Max(hi, row.Center), max(radius, row.Radius)
	}
	pad := box2d.Vec2{X: radius, Y: radius}
	box := box2d.AABB{LowerBound: box2d.Sub(lo, pad), UpperBound: box2d.Add(hi, pad)}
	r.tree.CreateProxy(box, uint64(now.Kind), key)
}

// View is the view a shot a client predicts is judged at on tick: the one its client drew, held to those
// whose tick's views (Across) are drawn from the rows kept by then, RewindTicks before tick up to tick - 2,
// whose views run to the newest row (and 1 at least, as rows start at tick 0). An older view is judged
// at the oldest, so a high ping must lead its shots but still hits. A newer one, or one that isn't a
// number, is judged at the newest.
func (t Tuning) View(view float64, tick uint64) float64 {
	newest := max(float64(tick)-2, 1)
	if !(view <= newest) {
		return newest
	}
	return max(view, float64(tick)-float64(t.RewindTicks()), 1)
}

// Fly flies f, a shot of type def from side, one tick of dt seconds. It meets the map in world as it is,
// and each target as sights shows it across the tick: one query of reach finds the targets its path can
// meet, however they moved, and a pair cast in each one's moving frame finds when they touch. With no
// reach, it meets no target. It returns f after the tick, or over=true and how it ended in it: at its
// range, or at the first thing on its path that its rules don't pass. A shot launched this tick flies
// this tick, so its path starts at the shooter.
func (t Tuning) Fly(world *box2d.World, def Shot, f Flight, side interact.Side, reach *Reach, sights Sights,
	dt float64,
) (next Flight, end End, over bool) {
	to, flown, seconds, last := f.step(dt)
	first, found := t.firstHit(world, def, f.Pos, to, f.Radius) // the map
	if reach != nil && f.Radius > 0 && f.Pos != to {
		part := seconds / dt
		pad := box2d.Vec2{X: f.Radius, Y: f.Radius}
		path := box2d.AABB{LowerBound: box2d.Sub(box2d.Min(f.Pos, to), pad), UpperBound: box2d.Add(box2d.Max(f.Pos, to), pad)}
		reach.tree.Query(path, uint64(targetKinds), func(_ int, key uint64, _ any) bool {
			if s, ok := sights(key); ok {
				if h, ok := t.hitOn(def, f, to, part, side, s); ok && (!found || h.before(first)) {
					first, found = h, true
				}
			}
			return true
		}, nil)
	}
	return f.advance(to, flown, seconds, dt, first, found, last)
}

// hitOn is where f, flying to to in part of the tick, first touches s, from a Box2D pair cast in s's
// frame as it moves in each span: the shot's move less s's. False when it misses s or the rules pass it.
func (t Tuning) hitOn(def Shot, f Flight, to box2d.Vec2, part float64, side interact.Side, s Seen) (hit, bool) {
	var first hit
	found := false
	for _, sp := range s.Spans {
		a, b := sp.Start/part, sp.End/part // of the shot's path
		if !(b > a) || a >= 1 {
			continue
		}
		m, i := t.meet(def, sp.Kind, side.Relation(sp.Side), sp.Tags)
		if m.Do == Pass {
			continue
		}
		from := lerp(f.Pos, to, a)
		move := box2d.Sub(lerp(f.Pos, to, b), from)
		out := box2d.ShapeCastCircle(&box2d.Circle{Radius: sp.Radius}, &box2d.ShapeCastInput{
			Proxy:       box2d.MakeProxy([]box2d.Vec2{box2d.Sub(from, sp.Center)}, 1, f.Radius),
			Translation: box2d.Sub(move, box2d.Sub(sp.To, sp.Center)),
			MaxFraction: min((1-a)/(b-a), 1), // to the path's end
		})
		if !out.Hit {
			continue
		}
		h := hit{fraction: a + float64(out.Fraction*(b-a)), rule: i, kind: sp.Kind, target: s.ID, serial: s.Serial,
			center: lerp(sp.Center, sp.To, out.Fraction), meet: m}
		if !found || h.before(first) {
			first, found = h, true
		}
	}
	return first, found
}
