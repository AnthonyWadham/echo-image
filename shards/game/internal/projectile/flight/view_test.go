package flight

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/track/past"
)

// A shot meets each Body and Echo as the tick shows it: when the two first touch as both move, wherever
// the target is now. Its end is from the target's center then.
func TestFlyMeetsEachTargetAsTheTickShowsIt(t *testing.T) {
	still := func(kind interact.Kind, side interact.Side, at, now box2d.Vec2) thing {
		return thing{kind, side, at, at, now}
	}
	for _, c := range []struct {
		name      string
		remaining float64
		wall      bool // across the path, 0.35 along
		things    []thing
		hit       uint32 // the thing it ends on, 10 + its index, or 0 for none or the wall
	}{
		{"on the path, off it now", 20, false, []thing{still(interact.Echo, enemy, onPath(0.5, 0), onPath(0.5, 3))}, 10},
		{"on the path, 40 units off it now (a blink)", 20, false,
			[]thing{still(interact.Echo, enemy, onPath(0.5, 0), onPath(0.5, 40))}, 10},
		{"off the path, on it now", 20, false, []thing{still(interact.Echo, enemy, onPath(0.5, 3), onPath(0.5, 0))}, 0},
		{"the nearer comes first", 20, false, []thing{
			still(interact.Echo, enemy, onPath(0.7, 0.2), onPath(-0.2, 0)),
			still(interact.Echo, enemy, onPath(0.9, -0.1), onPath(0.6, 0)),
		}, 10},
		{"an ally passes, the enemy behind it stops", 20, false, []thing{
			still(interact.Body, ally, onPath(0.2, 0), onPath(0.2, 0)),
			still(interact.Echo, enemy, onPath(0.6, 0.3), onPath(0.6, 2)),
		}, 11},
		{"a wall before it", 20, true, []thing{still(interact.Echo, enemy, onPath(1.2, 0), onPath(0.1, 0))}, 0},
		{"a Body on its Echo shields it", 20, false, []thing{
			still(interact.Echo, enemy, onPath(0.5, 0), onPath(0.5, -1)),
			still(interact.Body, enemy, onPath(0.5, 0), onPath(0.5, 1)),
		}, 11},
		{"past its range in a last tick", 0.3, false, []thing{still(interact.Echo, enemy, onPath(1.2, 0), onPath(1.2, 0))}, 0},
		{"crosses the path past a last tick's range", 0.3, false,
			[]thing{{interact.Echo, enemy, onPath(0.6, -2.4), onPath(0.6, 0.6), onPath(0.6, 0.6)}}, 0},
		// The shot flies 0.75 units in the tick.
		{"crosses the path as the shot gets there", 20, false,
			[]thing{{interact.Echo, enemy, onPath(0.5, -1.5), onPath(0.5, 1.5), onPath(0.5, 1.5)}}, 10},
		{"leaves the path before the shot gets there", 20, false,
			[]thing{{interact.Echo, enemy, onPath(0.7, 0), onPath(0.7, 3), onPath(0.7, 3)}}, 0},
		{"a dash across the path, both ticks' poses off it", 20, false,
			[]thing{{interact.Echo, enemy, onPath(0.4, -0.9), onPath(0.4, 0.9), onPath(0.4, 0.9)}}, 10},
	} {
		end := flyPast(c.remaining, c.wall, c.things...)
		var offset box2d.Vec2 // from its center when they met
		if c.hit != 0 {
			th := c.things[c.hit-10]
			offset = box2d.Sub(end.At, lerp(th.from, th.to, end.Until))
		}
		if end.Hit.Target != c.hit || end.Hit.Offset != offset {
			t.Errorf("%s: ended on %d at %v from its center, want %d at %v", c.name, end.Hit.Target, end.Hit.Offset,
				c.hit, offset)
		}
	}
	hits, things := 0, sweep()
	for i, th := range things {
		end := flyPast(20, false, th)
		touches(t, fmt.Sprint("sweep ", i), th, end)
		if end.Hit.Target != 0 {
			hits++
		}
	}
	if hits < 10 || len(things)-hits < 10 {
		t.Errorf("the sweep hit %d of %d: it should hit and miss often", hits, len(things))
	}
}

// Any enemy Echo, moving anywhere across a tick in an arena, meets the shot when the two first touch.
// The sweep is the seed corpus.
func FuzzFlyMeetsAMovingTargetWhenTheyTouch(f *testing.F) {
	for _, th := range sweep() {
		f.Add(th.from.X, th.from.Y, th.to.X, th.to.Y)
	}
	f.Fuzz(func(t *testing.T, fromX, fromY, toX, toY float64) {
		for _, v := range []float64{fromX, fromY, toX, toY} {
			if !(math.Abs(v) <= 100) { // an arena, and never NaN
				t.Skip()
			}
		}
		from, to := box2d.Vec2{X: fromX, Y: fromY}, box2d.Vec2{X: toX, Y: toY}
		th := thing{interact.Echo, enemy, from, to, to}
		touches(t, "fuzz", th, flyPast(20, false, th))
	})
}

// touches fails t unless end, the shot's against th alone, is when the two first touch, to Box2D's slop:
// it stops on th when their centers come within the sum of their radii, and not before they are deeper.
func touches(t testing.TB, name string, th thing, end End) {
	t.Helper()
	const r = 0.5 + 0.15
	slop := box2d.LinearSlop
	// The two centers apart at λ of the tick: start less from, then the shot's move less th's, a line.
	p := box2d.Sub(start, th.from)
	q := box2d.Sub(box2d.Vec2{X: dir.X * viewShot.Speed * 0.05, Y: dir.Y * viewShot.Speed * 0.05}, box2d.Sub(th.to, th.from))
	at := func(l float64) float64 { return math.Sqrt(math.Pow(p.X+q.X*l, 2) + math.Pow(p.Y+q.Y*l, 2)) }
	// first is the first λ in the tick they are within d, or 2 if never.
	first := func(d float64) float64 {
		qq, pq, pp := q.X*q.X+q.Y*q.Y, p.X*q.X+p.Y*q.Y, p.X*p.X+p.Y*p.Y-d*d
		if pp <= 0 {
			return 0
		}
		if disc := pq*pq - qq*pp; qq > 0 && disc >= 0 {
			if l := (-pq - math.Sqrt(disc)) / qq; l >= 0 && l <= 1 {
				return l
			}
		}
		return 2
	}
	switch hit := end.Hit.Target != 0; {
	case hit && (at(end.Until) > r || end.Until > first(r-2*slop)):
		t.Errorf("%s: met at %v of the tick, %v apart, want when they first touch, %v apart, by %v", name, end.Until,
			at(end.Until), r, first(r-2*slop))
	case !hit && first(r-2*slop) <= 1:
		t.Errorf("%s: missed, though they are %v deep at %v of the tick", name, r-at(first(r-2*slop)), first(r-2*slop))
	}
}

// The view tests' shot flies from start along dir at 20 Hz, under the GDD's default rules.
var (
	viewRules = func() (tuning Tuning) {
		if err := json.Unmarshal([]byte(`{"defaults": [
			{"kinds": ["wall", "bounds"], "do": "stop"},
			{"relations": ["own", "ally"], "do": "pass"},
			{"kinds": ["body"], "do": "stop"},
			{"kinds": ["echo"], "relations": ["enemy"], "do": "stop", "payload": true}]}`), &tuning); err != nil {
			panic(err)
		}
		return tuning
	}()
	viewShot    = Shot{Speed: 15, Range: 20, Radius: 0.15}
	start, dir  = box2d.Vec2{X: 1.25, Y: -0.5}, box2d.Vec2{X: 0.6, Y: 0.8}
	shooter     = interact.Side{Owner: 1, Team: 1}
	ally, enemy = interact.Side{Owner: 3, Team: 1}, interact.Side{Owner: 2, Team: 2}
)

// onPath is k along the shot's path from start, left of it.
func onPath(k, left float64) box2d.Vec2 {
	return box2d.Vec2{X: start.X + dir.X*k - dir.Y*left, Y: start.Y + dir.Y*k + dir.X*left}
}

// thing is a Body or Echo moving from from to to across the tick, and where it is now.
type thing struct {
	kind          interact.Kind
	side          interact.Side
	from, to, now box2d.Vec2
}

// sweep is 60 enemy Echoes the shot hits, grazes, misses or starts inside as the tick begins, each moving
// 2.5 units across it.
func sweep() []thing {
	var things []thing
	for i := range 60 {
		from := onPath(-0.3+float64(i)*0.0217, float64(i*7%13-6)*0.1625)
		to := box2d.Vec2{X: from.X + 2.5*math.Cos(float64(i)), Y: from.Y + 2.5*math.Sin(float64(i))}
		things = append(things, thing{interact.Echo, enemy, from, to, to})
	}
	return things
}

// flyPast flies the shot one tick, with remaining units to go and, if wall, a wall across its path 0.35
// along, past things.
func flyPast(remaining float64, wall bool, things ...thing) End {
	world := testWorld(wall, onPath(0.35, 0), dir)
	reach, seen := NewReach(), map[uint64]Seen{}
	for i, th := range things {
		key := uint64(10 + i)
		row := past.Row{Kind: th.kind, Side: th.side, Center: th.from, Radius: 0.5}
		reach.Add(key, past.Row{Kind: th.kind, Side: th.side, Center: th.now, Radius: 0.5}, row,
			past.Row{Center: th.to, Radius: 0.5})
		seen[key] = Seen{ID: uint32(key), Spans: [2]past.Span{{Row: row, End: 1, To: th.to}}}
	}
	f := Flight{Pos: start, Velocity: box2d.Vec2{X: dir.X * viewShot.Speed, Y: dir.Y * viewShot.Speed},
		Radius: viewShot.Radius, Remaining: remaining}
	sights := func(key uint64) (Seen, bool) { s, ok := seen[key]; return s, ok }
	_, end, _ := viewRules.Fly(world, viewShot, f, shooter, reach, sights, 0.05)
	return end
}

// A target is met where the tick shows it, at its size there, however far it moved and grew since.
func TestFlyMeetsTargetsWhereverTheyWent(t *testing.T) {
	var tuning Tuning
	if err := json.Unmarshal([]byte(`{"defaults": [{"kinds": ["echo"], "do": "stop"}]}`), &tuning); err != nil {
		t.Fatal(err)
	}
	then := past.Row{Kind: interact.Echo, Side: enemy, Center: box2d.Vec2{X: 0, Y: 1.2}, Radius: 1.5}
	reach := NewReach()
	reach.Add(10, past.Row{Kind: interact.Echo, Side: enemy, Center: box2d.Vec2{X: 50, Y: 50}, Radius: 0.5}, then)
	sights := func(key uint64) (Seen, bool) {
		return Seen{ID: uint32(key), Spans: [2]past.Span{{Row: then, End: 1, To: then.Center}}}, key == 10
	}
	f := Flight{Pos: box2d.Vec2{X: -2}, Velocity: box2d.Vec2{X: 40}, Radius: 0.15, Remaining: 20}
	def := Shot{Speed: 40, Range: 20, Radius: 0.15}
	world := testWorld(false, box2d.Vec2{}, box2d.Vec2{X: 1})
	_, end, over := tuning.Fly(world, def, f, shooter, reach, sights, 0.05)
	if !over || end.Hit.Target != 10 {
		t.Errorf("the shot ended %v on %d, want on 10, 1.2 from the center it passed and 1.5 wide then", over,
			end.Hit.Target)
	}
}

// testWorld is a world with, if wall, a thin wall at at, across direction dir.
func testWorld(wall bool, at, dir box2d.Vec2) *box2d.World {
	def := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&def)
	if wall {
		body := world.CreateBody(new(box2d.DefaultBodyDef()))
		shape := box2d.DefaultShapeDef()
		shape.Filter = box2d.Filter{CategoryBits: uint64(interact.Wall), MaskBits: uint64(interact.Shot)}
		box := box2d.MakeOffsetBox(0.05, 2, at, box2d.Rot{C: dir.X, S: dir.Y})
		world.CreatePolygonShape(body, &shape, &box)
	}
	return world
}
