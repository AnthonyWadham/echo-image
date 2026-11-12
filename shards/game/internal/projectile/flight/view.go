package flight

import (
	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/history/past"
)

// Fly flies f, a shot of type def that actor a fired, one tick of dt seconds. It meets the map in world
// as it is, and each recorded thing in scene as a saw it (past.Scene.Cast); with no scene, it meets no
// thing. It returns f after the tick, or over=true and how it ended in it: at its range, or at the
// first thing on its path that its rules don't pass. A shot launched this tick flies this tick, so its
// path starts at the shooter.
func (t Tuning) Fly(world *box2d.World, def Shot, f Flight, a past.Actor, scene *past.Scene, dt float64) (
	next Flight, end End, over bool,
) {
	to, flown, seconds, last := f.step(dt)
	first, found := t.firstHit(world, def, f.Pos, to, f.Radius) // the map
	if scene != nil && f.Radius > 0 && f.Pos != to {
		q := past.Query{Start: f.Pos, End: to, To: seconds / dt, Kinds: meetable}
		q.Shape.Count, q.Shape.Radius = 1, f.Radius
		meet := func(r past.Row) (int, bool) {
			m, i := t.meet(def, r.Kind, a.Side.Relation(r.Side), r.Tags)
			return i, m.Do != Pass
		}
		var room [4]past.Contact
		if c := scene.Cast(a, q, meet, room[:0]); len(c) > 0 {
			h := hit{fraction: c[0].Fraction, rule: c[0].Rank, kind: c[0].Row.Kind, target: c[0].ID,
				serial: c[0].Serial, center: c[0].Row.Center, meet: t.rule(def, c[0].Rank)}
			if !found || h.before(first) {
				first, found = h, true
			}
		}
	}
	next, end, over = f.advance(to, flown, seconds, dt, first, found, last)
	end.View = a.View
	return next, end, over
}
