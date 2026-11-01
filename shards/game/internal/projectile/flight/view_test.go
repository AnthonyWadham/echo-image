package flight

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics/timestep"
)

// A target at a view is drawn as the client draws it: its newer row, partway from its older one. One
// that died in the newer tick moves at its own pace to where it died, and is gone after. Every view in
// (10, 11] is drawn from rows 10 and 11: a whole tick is that tick's end.
func TestAtViewDrawsAsTheClientDoes(t *testing.T) {
	older := Row{Tick: 10, Center: box2d.Vec2{X: 1, Y: 2}, Radius: 0.5, Tags: interact.Stunned, Until: 1}
	at := func(x, y float64) *box2d.Vec2 { return &box2d.Vec2{X: x, Y: y} }
	for _, c := range []struct {
		name     string
		hasOlder bool
		until    float64 // the newer row's
		view     float64
		want     *box2d.Vec2 // nil if gone
	}{
		{"a quarter of the way", true, 1, 10.25, at(1.5, 1)},
		{"at the newer tick", true, 1, 11, at(3, -2)},
		{"new at the newer tick", false, 1, 10.25, at(3, -2)},
		{"a quarter of the way, dying halfway", true, 0.5, 10.25, at(2, 0)},
		{"where it died", true, 0.5, 10.5, at(3, -2)},
		{"after it died", true, 0.5, 10.75, nil},
		{"dead as the tick began", true, 0, math.Nextafter(10, 11), nil},
		{"new, after it died", false, 0.5, 10.75, nil},
	} {
		if a, b := RowTicks(c.view); a != 10 || b != 11 {
			t.Fatalf("%s: RowTicks(%v) = %d, %d, want 10, 11", c.name, c.view, a, b)
		}
		newer := Row{Tick: 11, Center: box2d.Vec2{X: 3, Y: -2}, Radius: 0.5, Tags: interact.CC, Until: c.until}
		got, ok := AtView(older, c.hasOlder, newer, c.view)
		want := newer
		if c.want != nil {
			want.Center = *c.want
		}
		if ok != (c.want != nil) || ok && got != want {
			t.Errorf("%s: %+v %v, want %+v %v", c.name, got, ok, want, c.want != nil)
		}
	}
}

// A predicted shot is judged at its client's view, held to the views the rows kept by its tick are drawn
// from: the oldest RewindTicks before it, whose older row is the oldest kept, and the newest at the
// newest row. A view that isn't a number is judged at the newest.
func TestViewIsHeldInsideTheRows(t *testing.T) {
	tuning := Tuning{Rewind: Rewind{InterpolationSeconds: 0.1, QueueSeconds: 0.05, PingSeconds: 0.25}}
	if got, want := tuning.RewindTicks(), uint64(timestep.Ticks(0.4)); got != want {
		t.Fatalf("RewindTicks = %d, want %d: its parts added up", got, want)
	}
	tick := uint64(100)
	oldest := float64(tick - tuning.RewindTicks())
	newest := float64(tick - 1) // drawn from rows 98 and 99, the newest
	for _, c := range []struct {
		view, want float64
	}{
		{oldest, oldest},
		{oldest - 3.5, oldest},
		{98.5, 98.5},
		{99, newest},
		{99.5, newest}, // needs row 100
		{math.NaN(), newest},
		{math.Inf(1), newest},
		{math.Inf(-1), oldest},
	} {
		if got := tuning.View(c.view, tick); got != c.want {
			t.Errorf("View(%v, %d) = %v, want %v", c.view, tick, got, c.want)
		}
	}
	if a, _ := RowTicks(oldest); a != tick-tuning.KeptRows() {
		t.Errorf("the oldest view is drawn from row %d, want the oldest kept, %d", a, tick-tuning.KeptRows())
	}
	if got := tuning.View(-0.5, 3); got != 1 { // rows start at tick 0
		t.Errorf("View(-0.5, 3) = %v, want 1", got)
	}
}

// FlyAtView flies a shot as Fly does through a world with each Body and Echo where the view drew it, to
// the bit: that is how the client predicts it. The server's world has them where they are now, any
// distance away.
func TestFlyAtViewMatchesFlyBitForBit(t *testing.T) {
	for _, c := range []struct {
		name      string
		remaining float64
		wall      bool // across the path, 0.35 along
		things    []thing
		hit       uint32 // the thing it ends on, 10 + its index, or 0 for none or the wall
	}{
		{"on the path in the view, off it now", 20, false,
			[]thing{{interact.Echo, enemy, along(0.5, 0), along(0.5, 3)}}, 10},
		{"on the path in the view, 40 units off it now (a blink)", 20, false,
			[]thing{{interact.Echo, enemy, along(0.5, 0), along(0.5, 40)}}, 10},
		{"off the path in the view, on it now", 20, false,
			[]thing{{interact.Echo, enemy, along(0.5, 3), along(0.5, 0)}}, 0},
		{"the nearer in the view comes first", 20, false, []thing{
			{interact.Echo, enemy, along(0.7, 0.2), along(-0.2, 0)},
			{interact.Echo, enemy, along(0.9, -0.1), along(0.6, 0)},
		}, 10},
		{"an ally passes, the enemy behind it stops", 20, false, []thing{
			{interact.Body, ally, along(0.2, 0), along(0.2, 0)},
			{interact.Echo, enemy, along(0.6, 0.3), along(0.6, 2)},
		}, 11},
		{"a wall before it in the view", 20, true,
			[]thing{{interact.Echo, enemy, along(1.2, 0), along(0.1, 0)}}, 0},
		{"a Body on its Echo shields it", 20, false, []thing{
			{interact.Echo, enemy, along(0.5, 0), along(0.5, -1)},
			{interact.Body, enemy, along(0.5, 0), along(0.5, 1)},
		}, 11},
		{"past its range in a last tick", 0.3, false,
			[]thing{{interact.Echo, enemy, along(1.2, 0), along(1.2, 0)}}, 0},
	} {
		end := flyBoth(t, c.name, c.remaining, c.wall, c.things...)
		var offset box2d.Vec2 // from the center it met, where the view drew it
		if c.hit != 0 {
			at := c.things[c.hit-10].view
			offset = box2d.Vec2{X: end.At.X - at.X, Y: end.At.Y - at.Y}
		}
		if end.Hit.Target != c.hit || end.Hit.Offset != offset {
			t.Errorf("%s: ended on %d at %v from its center, want %d at %v", c.name, end.Hit.Target, end.Hit.Offset,
				c.hit, offset)
		}
	}
	hits, things := 0, sweep()
	for i, th := range things {
		if flyBoth(t, fmt.Sprint("sweep ", i), 20, false, th).Hit.Target != 0 {
			hits++
		}
	}
	if hits < 10 || len(things)-hits < 10 {
		t.Errorf("the sweep hit %d of %d: it should hit and miss often", hits, len(things))
	}
}

// Any enemy Echo, drawn anywhere in an arena and now anywhere else, meets the shot the same both ways.
// The sweep is the seed corpus.
func FuzzFlyAtViewMatchesFly(f *testing.F) {
	for _, th := range sweep() {
		f.Add(th.view.X, th.view.Y, th.now.X, th.now.Y)
	}
	f.Fuzz(func(t *testing.T, viewX, viewY, nowX, nowY float64) {
		for _, v := range []float64{viewX, viewY, nowX, nowY} {
			if !(math.Abs(v) <= 100) { // an arena, and never NaN
				t.Skip()
			}
		}
		flyBoth(t, "fuzz", 20, false, thing{interact.Echo, enemy, box2d.Vec2{X: viewX, Y: viewY},
			box2d.Vec2{X: nowX, Y: nowY}})
	})
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

// along is k along the shot's path from start, left of it.
func along(k, left float64) box2d.Vec2 {
	return box2d.Vec2{X: start.X + dir.X*k - dir.Y*left, Y: start.Y + dir.Y*k + dir.X*left}
}

// thing is a Body or Echo where the view drew it and where it is now.
type thing struct {
	kind      interact.Kind
	side      interact.Side
	view, now box2d.Vec2
}

// sweep is 60 enemy Echoes the shot hits, grazes, misses or starts inside in the view, each 2.5 units
// from there now.
func sweep() []thing {
	var things []thing
	for i := range 60 {
		view := along(-0.3+float64(i)*0.0217, float64(i*7%13-6)*0.1625)
		now := box2d.Vec2{X: view.X + 2.5*math.Cos(float64(i)), Y: view.Y + 2.5*math.Sin(float64(i))}
		things = append(things, thing{interact.Echo, enemy, view, now})
	}
	return things
}

// flyBoth flies the shot both ways, with remaining units to go and, if wall, a wall across its path 0.35
// along. It fails t unless they agree to the bit, so the server ends it where its client did.
func flyBoth(t testing.TB, name string, remaining float64, wall bool, things ...thing) End {
	t.Helper()
	server, client := testWorld(wall, along(0.35, 0), dir), testWorld(wall, along(0.35, 0), dir)
	reach, seen, placed := NewReach(), map[uint32]Seen{}, map[box2d.ShapeID]Target{}
	for i, th := range things {
		target := Target{ID: uint32(10 + i), Side: th.side}
		row := Row{Kind: th.kind, Side: th.side, Center: th.view, Radius: 0.5}
		addCircle(server, th.kind, th.now)
		reach.Add(target.ID, Row{Kind: th.kind, Side: th.side, Center: th.now, Radius: 0.5}, row)
		seen[target.ID] = Seen{ID: target.ID, Row: row}
		placed[addCircle(client, th.kind, th.view)] = target
	}
	f := Flight{Pos: start, Velocity: box2d.Vec2{X: dir.X * viewShot.Speed, Y: dir.Y * viewShot.Speed},
		Radius: viewShot.Radius, Remaining: remaining}
	sights := func(id uint32) (Seen, bool) { v, ok := seen[id]; return v, ok }
	targets := func(s box2d.ShapeID) (Target, bool) { v, ok := placed[s]; return v, ok }
	const dt = 0.05
	next, end, over := viewRules.FlyAtView(server, viewShot, f, shooter, reach, sights, dt)
	wantNext, wantEnd, wantOver := viewRules.Fly(client, viewShot, f, shooter, targets, dt)
	if next != wantNext || end != wantEnd || over != wantOver {
		t.Errorf("%s: FlyAtView %+v %+v %v, Fly %+v %+v %v", name, next, end, over, wantNext, wantEnd, wantOver)
	}
	return end
}

// A target is met where the view had it, at its size there, however far it moved and grew since.
func TestFlyAtViewMeetsTargetsWhereverTheyWent(t *testing.T) {
	var tuning Tuning
	if err := json.Unmarshal([]byte(`{"defaults": [{"kinds": ["echo"], "do": "stop"}]}`), &tuning); err != nil {
		t.Fatal(err)
	}
	then := Row{Kind: interact.Echo, Side: enemy, Center: box2d.Vec2{X: 0, Y: 1.2}, Radius: 1.5}
	reach := NewReach()
	reach.Add(10, Row{Kind: interact.Echo, Side: enemy, Center: box2d.Vec2{X: 50, Y: 50}, Radius: 0.5}, then)
	sights := func(id uint32) (Seen, bool) { return Seen{ID: id, Row: then}, id == 10 }
	f := Flight{Pos: box2d.Vec2{X: -2}, Velocity: box2d.Vec2{X: 40}, Radius: 0.15, Remaining: 20}
	def := Shot{Speed: 40, Range: 20, Radius: 0.15}
	world := testWorld(false, box2d.Vec2{}, box2d.Vec2{X: 1})
	_, end, over := tuning.FlyAtView(world, def, f, shooter, reach, sights, 0.05)
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

// addCircle adds a Body or Echo to world: a kinematic body at at with one circle of radius 0.5, as the
// server's and the predictor's are.
func addCircle(world *box2d.World, kind interact.Kind, at box2d.Vec2) box2d.ShapeID {
	def := box2d.DefaultBodyDef()
	def.Type = box2d.KinematicBody
	body := world.CreateBody(&def)
	shape := box2d.DefaultShapeDef()
	shape.Filter = box2d.Filter{CategoryBits: uint64(kind), MaskBits: uint64(interact.Shot)}
	id := world.CreateCircleShape(body, &shape, &box2d.Circle{Radius: 0.5})
	world.SetBodyTransform(body, at, box2d.RotIdentity)
	return id
}
