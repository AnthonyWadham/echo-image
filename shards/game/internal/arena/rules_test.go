package arena_test

import (
	"math"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
	"echo_image/shards/game/internal/arena"
)

const radius = 0.5 // a Body's

// pt is (x, y) in a map file's units.
func pt(x, y float64) level.Point {
	return level.Point{int32(math.Round(x * level.Unit)), int32(math.Round(y * level.Unit))}
}

// box is a solid of kind, centred at (x, y), with half-sides hx and hy.
func box(kind string, x, y, hx, hy float64) level.Solid {
	corners := []level.Point{pt(x-hx, y-hy), pt(x+hx, y-hy), pt(x+hx, y+hy), pt(x-hx, y+hy)}
	return level.Solid{Kind: kind, Polygon: corners}
}

// edge is the map edge around a square of half-side h, clockwise.
func edge(h float64) level.Loop {
	return loop("bounds", [2]float64{-h, -h}, [2]float64{-h, h}, [2]float64{h, h}, [2]float64{h, -h})
}

// loop is a loop of kind through points (x, y).
func loop(kind string, points ...[2]float64) level.Loop {
	l := level.Loop{Kind: kind}
	for _, p := range points {
		l.Points = append(l.Points, pt(p[0], p[1]))
	}
	return l
}

func spawn(team uint8, x, y float64) level.Object {
	return level.Object{Type: "spawn", At: pt(x, y), Props: arena.Spawn{Team: team}}
}

// field is a 40 x 40 m edge with a spawn each side of a wall between them.
func field() level.Map {
	return level.Map{
		Solids:  []level.Solid{box("wall", 0, 0, 0.5, 5)},
		Edges:   []level.Loop{edge(20)},
		Objects: []level.Object{spawn(0, -10, 0), spawn(1, 10, 0)},
	}
}

// built is m, its places numbered as a file's are, built into a world of its own.
func built(t testing.TB, m level.Map) (level.Map, *box2d.World) {
	t.Helper()
	src := uint32(0)
	for i := range m.Solids {
		src++
		m.Solids[i].Src = src
	}
	for _, loops := range [][]level.Loop{m.Outlines, m.Edges} {
		for i := range loops {
			src++
			loops[i].Src = src
		}
	}
	for i := range m.Objects {
		src++
		m.Objects[i].ID = src
	}
	def := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&def)
	if _, err := level.Build(world, m, arena.Schema()); err != nil {
		t.Fatal(err)
	}
	return m, world
}

func check(t testing.TB, m level.Map) error {
	t.Helper()
	m, world := built(t, m)
	return arena.CheckMap(m, world, radius)
}

// A map passes when a Body has room at each spawn, walks between them and never off the map, every open
// space is walked to, and from anywhere in a wall it lands where it walks. Each rule names where it fails.
func TestCheckMap(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(*level.Map)
		want []string // none: it passes
	}{
		{"the field", func(*level.Map) {}, nil},
		{"wall too thick to land out of, past the edge", func(m *level.Map) {
			m.Solids = append(m.Solids, box("wall", 28, 0, 7.5, 7.5))
		}, nil},
		{"no spawn", func(m *level.Map) { m.Objects = nil }, []string{"the map has no spawn"}},
		{"first spawn in a wall", func(m *level.Map) { m.Solids = append(m.Solids, box("wall", -10, 0, 1, 1)) },
			[]string{"the first spawn, at (-10, 0), has no room"}},
		{"spawn touching a wall", func(m *level.Map) { m.Solids = append(m.Solids, box("wall", 11, 0, 0.5, 1)) },
			[]string{"the spawn at (10, 0) has no room"}},
		{"spawn walled off", func(m *level.Map) { m.Solids[0] = box("wall", 0, 0, 0.5, 20) },
			[]string{"can't walk from the first spawn to the one at (10, 0)"}},
		{"no edge", func(m *level.Map) { m.Edges = nil }, []string{"walks off the map"}},
		{"edge with a gap", func(m *level.Map) {
			m.Edges = nil
			m.Solids = append(m.Solids, box("bounds", -20, 0, 0.5, 20), box("bounds", 20, 0, 0.5, 20),
				box("bounds", 0, 20, 20, 0.5), box("bounds", -10.75, -20, 9.75, 0.5),
				box("bounds", 10.75, -20, 9.75, 0.5)) // a 2 m gap
		}, []string{"walks off the map"}},
		{"hollow outline", func(m *level.Map) {
			m.Outlines = append(m.Outlines,
				loop("wall", [2]float64{0, 10}, [2]float64{3, 10}, [2]float64{3, 13}, [2]float64{0, 13}))
		}, []string{"no Body walks to the open space at (0.75, 10.75) and"}},
		{"room walled off", func(m *level.Map) { // 2 x 2 m inside walls 1 m thick: its walls land into it
			m.Solids = append(m.Solids, box("wall", 0, 10.5, 2, 0.5), box("wall", 0, 13.5, 2, 0.5),
				box("wall", -1.5, 12, 0.5, 1), box("wall", 1.5, 12, 0.5, 1))
		}, []string{"no Body walks to the open space at (-0.25, 11.75) and 8 more spots",
			"can't land where Bodies walk"}},
		{"wall too thick to land out of", func(m *level.Map) { m.Solids[0] = box("wall", 0, 0, 6.75, 6.75) },
			[]string{"a Body in the wall at (-1.25, -1.25) and 120 more spots can't land where Bodies walk"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := field()
			c.edit(&m)
			err := check(t, m)
			if len(c.want) == 0 && err != nil {
				t.Fatalf("it fails: %v", err)
			}
			for _, want := range c.want {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("got %v, want an error naming %q", err, want)
				}
			}
		})
	}
}

// The rules on a 100 x 100 m edge with nothing inside, the size of the shipped map: what each world test
// pays at boot.
func BenchmarkCheckMapOpen(b *testing.B) {
	m := level.Map{
		Edges:   []level.Loop{edge(50)},
		Objects: []level.Object{spawn(0, -5, 0), spawn(1, 5, 0)},
	}
	m, world := built(b, m)
	for b.Loop() {
		if err := arena.CheckMap(m, world, radius); err != nil {
			b.Fatal(err)
		}
	}
}

// The rules on the shipped edge around walls a Body lands out of: 1 m thick, and 6.5 x 6 m.
func BenchmarkCheckMapWalls(b *testing.B) {
	m := level.Map{
		Solids:  []level.Solid{box("wall", 0, 0, 0.5, 5), box("wall", -4.75, -10, 3.25, 3)},
		Edges:   []level.Loop{edge(50)},
		Objects: []level.Object{spawn(0, -5, 0), spawn(1, 5, 0)},
	}
	m, world := built(b, m)
	for b.Loop() {
		if err := arena.CheckMap(m, world, radius); err != nil {
			b.Fatal(err)
		}
	}
}
