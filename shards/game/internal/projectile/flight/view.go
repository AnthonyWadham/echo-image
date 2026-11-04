package flight

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/track/past"
)

// lerp is k of the way from a to b.
func lerp(a, b box2d.Vec2, k float64) box2d.Vec2 {
	return box2d.Vec2{X: a.X + float64((b.X-a.X)*k), Y: a.Y + float64((b.Y-a.Y)*k)}
}

// Seen is a target as a shot's tick meets it: its entity and serial, and the spans of the tick it shows
// in, one per pair of rows the tick is drawn from (past.Across).
type Seen struct {
	ID     uint32
	Serial uint64
	Spans  [2]past.Span
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
func (r *Reach) Add(key uint64, now past.Row, kept ...past.Row) {
	lo, hi, radius := now.Center, now.Center, now.Radius
	for _, row := range kept {
		lo, hi, radius = box2d.Min(lo, row.Center), box2d.Max(hi, row.Center), max(radius, row.Radius)
	}
	pad := box2d.Vec2{X: radius, Y: radius}
	box := box2d.AABB{LowerBound: box2d.Sub(lo, pad), UpperBound: box2d.Add(hi, pad)}
	r.tree.CreateProxy(box, uint64(now.Kind), key)
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
		reach.tree.Query(path, math.MaxUint64, func(_ int, key uint64, _ any) bool {
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
