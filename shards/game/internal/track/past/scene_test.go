package past

import (
	"math"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
)

var sceneWindow = Window{InterpolationSeconds: 0.1, QueueSeconds: 0.05, PingSeconds: 0.1}

// still is a thing that stays at at, from tick 0 to now.
func still(id uint32, side interact.Side, at box2d.Vec2, now uint64) (Thing, []Row) {
	row := Row{Kind: interact.Echo, Side: side, Center: at, Radius: 0.5, Until: 1}
	var kept []Row
	for tick := range now {
		row.Tick = tick
		kept = append(kept, row)
	}
	row.Tick = now
	return Thing{ID: id, Serial: uint64(id) * 10, Now: row, From: at, To: at}, kept
}

// crossing is a thing that moves from from at view's tick to to the tick after: as a client judged at
// view (a whole tick) draws its tick.
func crossing(id uint32, from, to box2d.Vec2, view uint64) (Thing, []Row) {
	row := Row{Kind: interact.Echo, Side: interact.Side{Owner: 9, Team: 2}, Radius: 0.5, Until: 1}
	a, b := row, row
	a.Tick, a.Center, b.Tick, b.Center = view, from, view+1, to
	return Thing{ID: id, Serial: uint64(id) * 10, Now: b, From: to, To: to}, []Row{a}
}

// circle is a still query of radius r at at over the whole tick.
func circle(at box2d.Vec2, r float64) Query {
	q := Query{Start: at, End: at, To: 1, Kinds: interact.Echo | interact.Body}
	q.Shape.Count, q.Shape.Radius = 1, r
	return q
}

func meetAll(Row) (int, bool) { return 0, true }

// A client's actions are judged at its view, held by the window, and only while the thing it plays is
// the one with its serial. An action no client predicts is judged at this tick, as every screen draws
// it.
func TestSceneJudgesEachActorAtItsView(t *testing.T) {
	s := NewScene(sceneWindow)
	s.Reset(10)
	s.AddClient(2, 20, 99)
	s.AddClient(1, 10, 7.5)
	s.AddClient(3, 30, math.NaN())
	s.Build()
	side := interact.Side{Owner: 1, Team: 1}
	for _, c := range []struct {
		name   string
		id     uint32
		serial uint64
		view   float64
		ok     bool
	}{
		{"its view", 1, 10, 7.5, true},
		{"a view too new", 2, 20, 8, true},
		{"a view that isn't a number", 3, 30, 8, true},
		{"another thing with its ID", 1, 11, 0, false},
		{"no client", 4, 40, 0, false},
	} {
		a, ok := s.Client(c.id, c.serial, side)
		if ok != c.ok || ok && a != (Actor{Side: side, View: c.view, Predicted: true}) {
			t.Errorf("%s: Client = %+v, %v; want view %v, %v", c.name, a, ok, c.view, c.ok)
		}
	}
	if a := s.Present(side); a != (Actor{Side: side, View: 9}) {
		t.Errorf("Present = %+v, want this tick's views, 9 on", a)
	}
	s.Reset(0)
	if a := s.Present(side); a.View != 0 {
		t.Errorf("Present at tick 0 = %+v, want view 0", a)
	}
}

// A thing its owner's client predicts moves as that client draws it, From to To, for that client's
// predicted actions only; every other actor meets it by its rows.
func TestSceneDrawsOwnThingsAsTheirClientPredicts(t *testing.T) {
	own := interact.Side{Owner: 1, Team: 1}
	for _, c := range []struct {
		name      string
		predicted bool
		actor     Actor
		meets     bool
	}{
		{"its client's", true, Actor{Side: own, View: 8, Predicted: true}, true},
		{"another client's", true, Actor{Side: interact.Side{Owner: 2, Team: 1}, View: 8, Predicted: true}, false},
		{"its owner's, not predicted", true, Actor{Side: own, View: 8}, false},
		{"its client's, the thing not predicted", false, Actor{Side: own, View: 8, Predicted: true}, false},
	} {
		s := NewScene(sceneWindow)
		s.Reset(10)
		th, kept := still(1, own, box2d.Vec2{X: 5}, 10) // its rows: away from the query
		th.Predicted, th.From, th.To = c.predicted, box2d.Vec2{X: -2}, box2d.Vec2{X: 2}
		s.Add(th, kept)
		s.Build()
		got := s.Cast(c.actor, circle(box2d.Vec2{}, 0.1), meetAll, nil)
		if len(got) == 1 != c.meets {
			t.Errorf("%s: contacts %+v, want a contact: %v", c.name, got, c.meets)
		}
	}
}

// Cast gives each thing's first contact once, the nearest first, then by rank, then by ID, whatever
// order the scene holds them in.
func TestSceneCastSortsContacts(t *testing.T) {
	q := circle(box2d.Vec2{X: -3}, 0.1)
	q.End = box2d.Vec2{X: 3}
	rank := func(r Row) (int, bool) { return int(r.Side.Owner), r.Side.Owner != 7 } // 7: passed
	for _, back := range []bool{false, true} {
		s := NewScene(sceneWindow)
		s.Reset(10)
		things := []struct {
			id    uint32
			x     float64
			owner uint32
		}{{5, 0, 1}, {4, 0, 1}, {3, 0, 0}, {9, -1, 2}, {8, -2, 7}}
		if back {
			slices.Reverse(things)
		}
		for _, th := range things {
			s.Add(still(th.id, interact.Side{Owner: th.owner, Team: 2}, box2d.Vec2{X: th.x}, 10))
		}
		s.Build()
		var ids []uint32
		for _, c := range s.Cast(Actor{Side: interact.Side{Owner: 1, Team: 1}, View: 8}, q, rank, nil) {
			ids = append(ids, c.ID)
		}
		if want := []uint32{9, 3, 4, 5}; !slices.Equal(ids, want) {
			t.Errorf("added last first %v: contacts %v, want %v", back, ids, want)
		}
	}
}

// A query over part of the tick meets a thing only where that part shows it: one that crossed it
// before is passed, one that crosses it then is met as it moves, and an instant meets each thing where
// that instant shows it.
func TestSceneCastCoversPartOfTheTick(t *testing.T) {
	s := NewScene(sceneWindow)
	s.Reset(10)
	early, kept := crossing(1, box2d.Vec2{X: -1, Y: 0}, box2d.Vec2{X: 3, Y: 0}, 8) // at the origin at 0.25
	s.Add(early, kept)
	late, kept := crossing(2, box2d.Vec2{X: -3, Y: 5}, box2d.Vec2{X: 1, Y: 5}, 8) // at y 5, x 0 at 0.75
	s.Add(late, kept)
	dead, kept := crossing(3, box2d.Vec2{}, box2d.Vec2{}, 8) // at the origin, gone at 0.4
	dead.Now.Until = 0.4
	s.Add(dead, kept)
	s.Build()
	a := Actor{Side: interact.Side{Owner: 1, Team: 1}, View: 8}
	// A still circle at the origin, then at y 5, over the tick's second half.
	for _, c := range []struct {
		name string
		at   box2d.Vec2
		id   uint32
		x, f float64 // where the thing was at the touch, and when, of the query's path
	}{
		{"crossed before", box2d.Vec2{}, 0, 0, 0},
		{"crosses then", box2d.Vec2{Y: 5}, 2, -0.6, 0.2}, // 0.6 apart at 0.6 of the tick
	} {
		q := circle(c.at, 0.1)
		q.From = 0.5
		got := s.Cast(a, q, meetAll, nil)
		switch {
		case c.id == 0 && len(got) != 0:
			t.Errorf("%s: contacts %+v, want none", c.name, got)
		case c.id != 0 && (len(got) != 1 || got[0].ID != c.id || math.Abs(got[0].Row.Center.X-c.x) > 2*box2d.LinearSlop ||
			math.Abs(got[0].Fraction-c.f) > 2*box2d.LinearSlop):
			t.Errorf("%s: contacts %+v, want %d at x %v, %v of the way", c.name, got, c.id, c.x, c.f)
		}
	}
	// A shot from y -3 to 7 at one instant: at 0.25 early and dead are on its line at the origin, and at
	// 0.75 late is, at y 5; at 0.5 early is at x 1, late at x -1, and dead is gone.
	for _, c := range []struct {
		at   float64
		ids  []uint32
		y, x float64 // the first contact's y along the line, and its thing's x
	}{
		{0.25, []uint32{1, 3}, -0.6, 0},
		{0.75, []uint32{2}, 4.4, 0},
		{0.5, nil, 0, 0},
	} {
		q := circle(box2d.Vec2{Y: -3}, 0.1)
		q.End, q.From, q.To = box2d.Vec2{Y: 7}, c.at, c.at
		got := s.Cast(a, q, meetAll, nil)
		var ids []uint32
		for _, g := range got {
			ids = append(ids, g.ID)
		}
		if !slices.Equal(ids, c.ids) || len(got) > 0 && (math.Abs(-3+10*got[0].Fraction-c.y) > 2*box2d.LinearSlop ||
			math.Abs(got[0].Row.Center.X-c.x) > 1e-12) {
			t.Errorf("instant %v: contacts %+v, want %v first at y %v, x %v", c.at, got, c.ids, c.y, c.x)
		}
	}
}

// Each tick's scene holds only what was added since its Reset.
func TestSceneResetsEachTick(t *testing.T) {
	s := NewScene(sceneWindow)
	s.Reset(10)
	s.Add(still(1, interact.Side{Owner: 9, Team: 2}, box2d.Vec2{}, 10))
	s.AddClient(1, 10, 8)
	s.Build()
	s.Reset(11)
	s.Build()
	a := Actor{Side: interact.Side{Owner: 1, Team: 1}, View: 9}
	if got := s.Cast(a, circle(box2d.Vec2{}, 0.1), meetAll, nil); len(got) != 0 {
		t.Errorf("a reset scene met %+v", got)
	}
	if _, ok := s.Client(1, 10, a.Side); ok {
		t.Error("a reset scene kept last tick's client")
	}
}

// BenchmarkSceneBuild builds a scene of 24 things over a 60 Hz window, as SceneSystem does each tick.
func BenchmarkSceneBuild(b *testing.B) {
	type entry struct {
		thing Thing
		kept  []Row
	}
	var things []entry
	for i := range 24 {
		th, kept := still(uint32(i), interact.Side{Owner: uint32(i), Team: uint8(i % 2)},
			box2d.Vec2{X: float64(i % 6), Y: float64(i / 6)}, 15)
		things = append(things, entry{th, kept})
	}
	s := NewScene(sceneWindow)
	b.ReportAllocs()
	for i := range b.N {
		s.Reset(uint64(15 + i%2))
		for _, e := range things {
			s.Add(e.thing, e.kept)
		}
		for i := range 6 {
			s.AddClient(uint32(i), uint64(i)*10, 13)
		}
		s.Build()
	}
}
