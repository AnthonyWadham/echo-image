package level_test

import (
	"flag"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
)

var update = flag.Bool("update", false, "rewrite the golden file from arena()")

const golden = "testdata/arena.level.json"

// spawn is the arena's one object type.
type spawn struct {
	Team int `json:"team"`
}

var schema = level.Schema{
	Kinds: level.Kinds{
		"wall":   {CategoryBits: 1, MaskBits: math.MaxUint64},
		"fill":   {CategoryBits: 2, MaskBits: math.MaxUint64},
		"bounds": {CategoryBits: 4, MaskBits: math.MaxUint64},
	},
	Types: level.Types{"spawn": reflect.TypeFor[spawn]()},
}

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

// arena is a small map with every shape and a spawn per team: a 20 x 12 m edge, an L-shaped wall as an
// outline and its fill, a box, a pillar and a capsule.
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
		Objects: []level.Object{
			{ID: 6, Type: "spawn", At: pt(-8, 0), Props: spawn{Team: 1}},
			{ID: 7, Type: "spawn", At: pt(8, 0), Props: spawn{Team: 2}},
		},
	}
}

// The canonical bytes are pinned: Encode writes the golden file, and Decode reads it back to the map
// that writes it again.
func TestEncodeWritesTheGoldenFile(t *testing.T) {
	data, err := level.Encode(arena(), schema)
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
	m, err := level.Decode(want, schema)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := level.EncodeUnchecked(m); err != nil || string(again) != string(want) {
		t.Fatalf("the decoded map encodes to\n%s (%v)", again, err)
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

// Decode refuses a file that isn't exactly Encode's bytes for its map, a kind or type the schema doesn't
// name, props that don't fit their struct, and a shape Box2D wouldn't build exactly as written. Each
// case breaks one rule of the arena.
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
		{"a kind the schema doesn't name", func(m *level.Map) { m.Solids[2].Kind = "glass" }, nil, `unknown kind "glass"`},
		{"a loop kind the schema doesn't name", func(m *level.Map) {
			m.Edges[0].Kind = "glass"
		}, nil, `unknown kind "glass"`},
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
		{"an outline that crosses itself", func(m *level.Map) {
			m.Outlines[0].Points = []level.Point{pt(-6, -2), pt(-2, -1), pt(-2, -2), pt(-6, -1)}
		}, nil, "segments 0 and 2 cross"},
		{"an outline with sides 4 mm apart", func(m *level.Map) {
			m.Outlines[0].Points = []level.Point{pt(-6, -2), pt(-2, -2), pt(-2, -1), pt(-4, -1), {u(-4), u(-2) + 4}}
		}, nil, "segments 0 and 3 cross or come within"},
		{"an edge folding back on itself", func(m *level.Map) {
			m.Edges[0].Points = append(m.Edges[0].Points, pt(0, -6), pt(5, -6))
		}, nil, "segments 3 and 5 cross"},
		{"an outline wound clockwise", func(m *level.Map) {
			m.Outlines[0].Points = reverse(m.Outlines[0].Points)
		}, nil, "wrong way"},
		{"an edge wound counter-clockwise", func(m *level.Map) {
			m.Edges[0].Points = reverse(m.Edges[0].Points)
		}, nil, "wrong way"},
		{"a point past Box2D's world", func(m *level.Map) {
			m.Edges[0].Points[2] = pt(100_001, 6)
		}, nil, "outside Box2D's world"},
		{"a type the schema doesn't name", func(m *level.Map) { m.Objects[0].Type = "chest" }, nil, `unknown type "chest"`},
		{"props with an unknown field", nil, replace(`{"team":1}`, `{"team":1,"hat":2}`), "unknown field"},
		{"props of the wrong JSON type", nil, replace(`{"team":1}`, `{"team":"red"}`), "props"},
		{"props missing a field", nil, replace(`{"team":1}`, `{}`), "canonical"},
		{"no props", nil, replace(`, "props": {"team":1}`, ``), "props"},
		{"an object id 0", func(m *level.Map) { m.Objects[0].ID = 0 }, nil, "id 0, want 1 or more"},
		{"two objects with one id", func(m *level.Map) { m.Objects[1].ID = 6 }, nil, "an earlier object has this id"},
		{"an object past Box2D's world", func(m *level.Map) {
			m.Objects[0].At = pt(100_001, 0)
		}, nil, "outside Box2D's world"},
	} {
		m := arena()
		if c.edit != nil {
			c.edit(&m)
		}
		data, err := level.EncodeUnchecked(m)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		text := string(data)
		if c.text != nil {
			text = c.text(text)
		}
		if _, err := level.Decode([]byte(text), schema); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
	}
}

// Encode refuses what Decode would, for a map made in code: an object of a type the schema doesn't name,
// or whose props aren't its type's struct.
func TestEncodeRefuses(t *testing.T) {
	type twin struct {
		Team int `json:"team"`
	}
	for _, c := range []struct {
		name string
		edit func(*level.Map)
		want string
	}{
		{"a type the schema doesn't name", func(m *level.Map) { m.Objects[0].Type = "chest" }, `unknown type "chest"`},
		{"props of another struct", func(m *level.Map) { m.Objects[0].Props = twin{Team: 1} }, "want a level_test.spawn"},
	} {
		m := arena()
		c.edit(&m)
		if _, err := level.Encode(m, schema); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
	}
}
