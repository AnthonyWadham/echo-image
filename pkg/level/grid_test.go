package level_test

import (
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
)

// blockers is a query filter that every shape of the test schema stops.
var blockers = box2d.QueryFilter{CategoryBits: 1, MaskBits: math.MaxUint64}

func newGrid(t testing.TB, m level.Map) (*level.Grid, *box2d.World) {
	t.Helper()
	world := newWorld()
	if _, err := level.Build(world, m, schema); err != nil {
		t.Fatal(err)
	}
	g, err := level.NewGrid(world, m, 0.25, 0.5, blockers)
	if err != nil {
		t.Fatal(err)
	}
	return g, world
}

// A cell is open exactly when a circle at its centre touches nothing, however many cells one AABB query
// cleared at once. The ring around the grid is open, and centres are whole multiples of a cell.
func TestGridOpensCellsWhereTheCircleTouchesNothing(t *testing.T) {
	g, world := newGrid(t, arena())
	open := 0
	for k := range g.Len() {
		c := g.Center(k)
		proxy := box2d.MakeProxy([]box2d.Vec2{c}, 1, 0.5)
		touches := false
		world.OverlapShape(&proxy, blockers, func(box2d.ShapeID, any) bool {
			touches = true
			return false
		}, nil)
		if g.Open(k) == touches {
			t.Fatalf("cell %d at %v: open %v, but a circle there touches a shape: %v", k, c, g.Open(k), touches)
		}
		if math.Mod(c.X, g.Cell()) != 0 || math.Mod(c.Y, g.Cell()) != 0 {
			t.Fatalf("cell %d centred at %v, off the %v m grid", k, c, g.Cell())
		}
		if g.Open(k) {
			open++
		}
	}
	for _, k := range g.Border() {
		if !g.Open(k) {
			t.Fatalf("border cell %d at %v is shut", k, g.Center(k))
		}
	}
	if open == 0 || open == g.Len() {
		t.Fatalf("%d of %d cells open, want some of each", open, g.Len())
	}
}

// A flood from a spawn stays inside the edge, and one from the ring around the grid stays out. Between
// them they reach every open cell: the test map has no pocket.
func TestFloodStaysOnItsSideOfTheEdge(t *testing.T) {
	m := arena()
	g, _ := newGrid(t, m)
	spawn, ok := g.Nearest(m.Objects[0].At.Vec2())
	if !ok {
		t.Fatal("the spawn is off the grid")
	}
	in, out := g.Flood([]int{spawn}), g.Flood(g.Border())
	if other, _ := g.Nearest(m.Objects[1].At.Vec2()); !in[other] {
		t.Error("a flood from one spawn doesn't reach the other")
	}
	for k := range g.Len() {
		switch {
		case in[k] && out[k]:
			t.Fatalf("cell %d at %v is reached from inside and outside the edge", k, g.Center(k))
		case g.Open(k) && !in[k] && !out[k]:
			t.Fatalf("open cell %d at %v is reached from neither side", k, g.Center(k))
		case !g.Open(k) && (in[k] || out[k]):
			t.Fatalf("shut cell %d at %v is reached", k, g.Center(k))
		}
	}
}

// Without its edge, the map leaks: a flood from a spawn reaches the ring around the grid.
func TestFloodLeaksWithoutAnEdge(t *testing.T) {
	m := arena()
	m.Edges = nil
	g, _ := newGrid(t, m)
	spawn, _ := g.Nearest(m.Objects[0].At.Vec2())
	in := g.Flood([]int{spawn})
	for _, k := range g.Border() {
		if in[k] {
			return
		}
	}
	t.Fatal("a flood from a spawn stays inside a map with no edge")
}

// A grid samples only circles at least half a cell wide, and only a map with something in it.
func TestNewGridRefuses(t *testing.T) {
	world := newWorld()
	if _, err := level.NewGrid(world, arena(), 0.25, 0.1, blockers); err == nil {
		t.Error("a 0.1 m circle on a 0.25 m grid: no error")
	}
	if _, err := level.NewGrid(world, level.Map{}, 0.25, 0.5, blockers); err == nil {
		t.Error("an empty map: no error")
	}
}

// A 100 x 100 m edge with nothing inside, the size of echo_image's open map: its AABB queries clear
// nearly every block at once.
func BenchmarkNewGridOpenMap(b *testing.B) {
	m := level.Map{Edges: []level.Loop{{Kind: "bounds", Points: []level.Point{
		pt(-50, -50), pt(-50, 50), pt(50, 50), pt(50, -50),
	}}}}
	world := newWorld()
	if _, err := level.Build(world, m, schema); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		g, err := level.NewGrid(world, m, 0.25, 0.5, blockers)
		if err != nil {
			b.Fatal(err)
		}
		g.Flood(g.Border())
	}
}
