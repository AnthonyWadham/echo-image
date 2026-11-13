package level_test

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
)

var update = flag.Bool("update", false, "rewrite the golden file from arena()")

const golden = "testdata/arena.level.json"

// u returns meters in units.
func u(meters float64) int32 { return int32(meters * level.Unit) }

func pt(x, y float64) level.Point { return level.Point{u(x), u(y)} }

func vecs(points []level.Point) []box2d.Vec2 {
	v := make([]box2d.Vec2, len(points))
	for i, p := range points {
		v[i] = p.Vec2()
	}
	return v
}

// convex returns points in the order box2d.ComputeHull gives them, as a baker writes a polygon.
func convex(points ...level.Point) []level.Point {
	hull := box2d.ComputeHull(vecs(points))
	out := make([]level.Point, hull.Count)
	for i := range out {
		out[i] = level.Point{int32(hull.Points[i].X * level.Unit), int32(hull.Points[i].Y * level.Unit)}
	}
	return out
}

// arena is a small map with every shape: a 20 x 12 m edge, an L-shaped wall as an outline and its
// fill, a box, a pillar and a capsule.
func arena() level.Map {
	return level.Map{
		Solids: []level.Solid{
			{Src: 2, Kind: "fill", Polygon: convex(pt(-6, -2), pt(-2, -2), pt(-2, -1), pt(-6, -1))},
			{Src: 2, Kind: "fill", Polygon: convex(pt(-6, -1), pt(-5, -1), pt(-5, 2), pt(-6, 2))},
			{Src: 3, Kind: "wall", Polygon: convex(pt(2, 1), pt(4, 1), pt(4, 2), pt(2, 2))},
			{Src: 4, Kind: "wall", Circle: &level.Circle{Center: pt(0, 3), Radius: u(0.5)}},
			{Src: 5, Kind: "wall", Capsule: &level.Capsule{Center1: pt(3, -3), Center2: pt(6, -3), Radius: u(0.25)}},
		},
		Outlines: []level.Loop{{Src: 2, Kind: "wall", Points: []level.Point{
			pt(-6, -2), pt(-2, -2), pt(-2, -1), pt(-5, -1), pt(-5, 2), pt(-6, 2),
		}}},
		Edges: []level.Loop{{Src: 1, Kind: "bounds", Points: []level.Point{
			pt(-10, -6), pt(-10, 6), pt(10, 6), pt(10, -6),
		}}},
	}
}

// The canonical bytes are pinned: Encode writes the golden file, and Decode reads it back to the map
// that writes it again.
func TestEncodeWritesTheGoldenFile(t *testing.T) {
	data, err := level.Encode(arena())
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(golden, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(want) {
		t.Fatalf("Encode wrote\n%s\nwant %s (go test -update rewrites it)", data, golden)
	}
	m, err := level.Decode(want)
	if err != nil {
		t.Fatal(err)
	}
	if again := level.EncodeUnchecked(m); string(again) != string(want) {
		t.Fatalf("the decoded map encodes to\n%s", again)
	}
}

func replace(old, with string) func(string) string {
	return func(s string) string { return strings.Replace(s, old, with, 1) }
}

func reverse(points []level.Point) []level.Point {
	out := make([]level.Point, len(points))
	for i, p := range points {
		out[len(points)-1-i] = p
	}
	return out
}

// Decode refuses a file that isn't exactly Encode's bytes for its map, and a shape Box2D wouldn't build
// exactly as written. Each case breaks one rule of the arena.
func TestDecodeRefuses(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(*level.Map)
		text func(string) string
		want string
	}{
		{"another format", nil, replace(`"format": 1`, `"format": 2`), "format 2, want 1"},
		{"an unknown key", nil, replace(`{"src": 3, `, `{"src": 3, "tag": 1, `), "unknown field"},
		{"a key twice", nil, replace(`{"src": 3, `, `{"src": 3, "src": 3, `), "canonical"},
		{"a key in capitals", nil, replace(`{"src": 3, `, `{"Src": 3, `), "canonical"},
		{"minus zero", nil, replace(`[0, 3072]`, `[-0, 3072]`), "canonical"},
		{"null for a list", func(m *level.Map) {
			m.Outlines = nil
		}, replace(`"outlines": []`, `"outlines": null`), "canonical"},
		{"a third number in a point", nil, replace(`[0, 3072]`, `[0, 3072, 1]`), "canonical"},
		{"other spacing", nil, replace(`"src": 3`, `"src":3`), "canonical"},
		{"bytes after the map", nil, func(s string) string { return s + "{}\n" }, "after top-level value"},
		{"a solid with no shape", func(m *level.Map) { m.Solids[2].Polygon = nil }, nil, "0 shapes"},
		{"a solid with two shapes", func(m *level.Map) { m.Solids[3].Polygon = m.Solids[2].Polygon }, nil, "2 shapes"},
		{"a polygon of 2 points", func(m *level.Map) {
			m.Solids[2].Polygon = m.Solids[2].Polygon[:2]
		}, nil, "2 points, want 3 to 8"},
		{"a concave polygon", func(m *level.Map) { m.Solids[0].Polygon = m.Outlines[0].Points }, nil, "isn't convex"},
		{"a polygon in another order", func(m *level.Map) {
			m.Solids[2].Polygon = reverse(m.Solids[2].Polygon)
		}, nil, "order"},
		{"a circle of radius 0", func(m *level.Map) { m.Solids[3].Circle.Radius = 0 }, nil, "radius 0"},
		{"a capsule 5 mm long", func(m *level.Map) {
			m.Solids[4].Capsule.Center2 = level.Point{u(3) + 5, u(-3)}
		}, nil, "write a circle"},
		{"a loop of 3 points", func(m *level.Map) { m.Edges[0].Points = m.Edges[0].Points[:3] }, nil, "3 points, want 4"},
		{"a 5 mm segment", func(m *level.Map) {
			m.Edges[0].Points = append(m.Edges[0].Points, level.Point{u(10) - 5, u(-6)})
		}, nil, "segment 3 is"},
		{"an outline wound clockwise", func(m *level.Map) {
			m.Outlines[0].Points = reverse(m.Outlines[0].Points)
		}, nil, "wrong way"},
		{"an edge wound counter-clockwise", func(m *level.Map) {
			m.Edges[0].Points = reverse(m.Edges[0].Points)
		}, nil, "wrong way"},
		{"a kind in capitals", func(m *level.Map) { m.Solids[2].Kind = "Wall" }, nil, `kind "Wall"`},
		{"a point past Box2D's world", func(m *level.Map) {
			m.Edges[0].Points[2] = pt(100_001, 6)
		}, nil, "outside Box2D's world"},
	} {
		m := arena()
		if c.edit != nil {
			c.edit(&m)
		}
		text := string(level.EncodeUnchecked(m))
		if c.text != nil {
			text = c.text(text)
		}
		if _, err := level.Decode([]byte(text)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
	}
}
