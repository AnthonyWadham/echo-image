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
// the target is now, whatever order the scene holds them in. Its end is from the target's center then.
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
		end := flyPast(c.remaining, c.wall, false, c.things...)
		var offset box2d.Vec2 // from its center when they met
		if c.hit != 0 {
			th := c.things[c.hit-10]
			offset = box2d.Sub(end.At, box2d.Add(th.from, box2d.MulSV(end.Until, box2d.Sub(th.to, th.from))))
		}
		if end.Hit.Target != c.hit || box2d.Distance(end.Hit.Offset, offset) > 1e-12 || end.View != judged {
			t.Errorf("%s: ended on %d at %v from its center, judged at %v, want %d at %v, at %v", c.name,
				end.Hit.Target, end.Hit.Offset, end.View, c.hit, offset, float64(judged))
		}
		if back := flyPast(c.remaining, c.wall, true, c.things...); back != end {
			t.Errorf("%s: the things added last first, it ended %+v, want %+v", c.name, back, end)
		}
	}
	hits, things := 0, sweep()
	for i, th := range things {
		end := flyPast(20, false, false, th)
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
		touches(t, "fuzz", th, flyPast(20, false, false, th))
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

// The view tests' shot flies from start along dir at 20 Hz, under the GDD's default rules, fired by a
// client whose screen showed tick judged as the tick began.
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
	viewer      = past.Actor{Side: shooter, View: judged, Predicted: true}
	window      = past.Window{InterpolationSeconds: 0.1, QueueSeconds: 0.05, PingSeconds: 0.1}
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

// judged is the tick the tests' shooter saw, 3 before the scene's: a thing moves from its row then to the
// next one's as the shot flies, and is elsewhere now.
const judged = 7

// sceneOf is a scene of tick judged + 3 holding each thing by ID 10 + its index, last first if back.
func sceneOf(back bool, things ...thing) *past.Scene {
	scene := past.NewScene(window)
	scene.Reset(judged + 3)
	for k := range things {
		i := k
		if back {
			i = len(things) - 1 - k
		}
		th := things[i]
		row := func(tick uint64, at box2d.Vec2) past.Row {
			return past.Row{Tick: tick, Kind: th.kind, Side: th.side, Center: at, Radius: 0.5, Until: 1}
		}
		scene.Add(past.Thing{ID: uint32(10 + i), Serial: uint64(10 + i), Now: row(judged+3, th.now), From: th.now,
			To: th.now}, []past.Row{row(judged, th.from), row(judged+1, th.to)})
	}
	scene.Build()
	return scene
}

// flyPast flies the shot one tick, with remaining units to go and, if wall, a wall across its path 0.35
// along, past things (sceneOf).
func flyPast(remaining float64, wall, back bool, things ...thing) End {
	world := testWorld(wall, onPath(0.35, 0), dir)
	f := Flight{Pos: start, Velocity: box2d.Vec2{X: dir.X * viewShot.Speed, Y: dir.Y * viewShot.Speed},
		Radius: viewShot.Radius, Remaining: remaining}
	_, end, _ := viewRules.Fly(world, viewShot, f, viewer, sceneOf(back, things...), 0.05)
	return end
}

// A target is met where the tick shows it, at its size there, however far it moved and grew since.
func TestFlyMeetsTargetsWhereverTheyWent(t *testing.T) {
	var tuning Tuning
	if err := json.Unmarshal([]byte(`{"defaults": [{"kinds": ["echo"], "do": "stop"}]}`), &tuning); err != nil {
		t.Fatal(err)
	}
	then := past.Row{Tick: judged, Kind: interact.Echo, Side: enemy, Center: box2d.Vec2{X: 0, Y: 1.2}, Radius: 1.5,
		Until: 1}
	next, now := then, past.Row{Tick: judged + 3, Kind: interact.Echo, Side: enemy, Center: box2d.Vec2{X: 50, Y: 50},
		Radius: 0.5, Until: 1}
	next.Tick++
	scene := past.NewScene(window)
	scene.Reset(judged + 3)
	scene.Add(past.Thing{ID: 10, Serial: 10, Now: now, From: now.Center, To: now.Center}, []past.Row{then, next})
	scene.Build()
	f := Flight{Pos: box2d.Vec2{X: -2}, Velocity: box2d.Vec2{X: 40}, Radius: 0.15, Remaining: 20}
	def := Shot{Speed: 40, Range: 20, Radius: 0.15}
	world := testWorld(false, box2d.Vec2{}, box2d.Vec2{X: 1})
	_, end, over := tuning.Fly(world, def, f, viewer, scene, 0.05)
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
