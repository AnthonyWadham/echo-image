package bake_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
	"echo_image/pkg/level/bake"
)

var update = flag.Bool("update", false, "rewrite the golden files from testdata/yard.tmj and config")

const source = "testdata/yard.tmj"

// spawn is the yard's one object type.
type spawn struct {
	Team uint8 `json:"team"`
}

var config = bake.Config{
	Schema: level.Schema{
		Kinds: level.Kinds{
			"wall":   {CategoryBits: 1, MaskBits: math.MaxUint64},
			"bounds": {CategoryBits: 2, MaskBits: math.MaxUint64},
		},
		Types: level.Types{"spawn": reflect.TypeFor[spawn]()},
	},
	Edges: []string{"bounds"},
}

// golden fails unless got is the file at path, or rewrites it with -update.
func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("got\n%s\nwant %s (go test -update rewrites it)", got, path)
	}
}

func yard(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// near reports whether a and b are within one unit.
func near(a, b box2d.Vec2) bool {
	return math.Abs(a.X-b.X) <= 1.0/level.Unit && math.Abs(a.Y-b.Y) <= 1.0/level.Unit
}

func at(x, y float64) box2d.Vec2 { return box2d.Vec2{X: x, Y: y} }

// The yard is a 20 x 12 m canvas of 32 px tiles: an edge around it, a wall of each shape, one turned 30°,
// a hidden capsule turned 90° and one drawn tall, and a spawn each side. It bakes to the golden file, in
// meters from the canvas's centre, y up, each object turned clockwise about its origin as Tiled draws it.
func TestBakeWritesTheGoldenFile(t *testing.T) {
	data, err := bake.Bake("yard", yard(t), config)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "testdata/yard.level.json", data)
	m, err := level.Decode(data, config.Schema)
	if err != nil {
		t.Fatal(err)
	}
	edge := m.Edges[0].Points
	if got := []box2d.Vec2{edge[0].Vec2(), edge[1].Vec2(), edge[2].Vec2(), edge[3].Vec2()}; !reflect.DeepEqual(got,
		[]box2d.Vec2{at(-10, -6), at(-10, 6), at(10, 6), at(10, -6)}) {
		t.Errorf("the edge is %v, want the canvas clockwise from its lowest left corner", got)
	}
	centre := func(points []level.Point) box2d.Vec2 {
		var c box2d.Vec2
		for _, p := range points {
			c = box2d.Vec2{X: c.X + p.Vec2().X/float64(len(points)), Y: c.Y + p.Vec2().Y/float64(len(points))}
		}
		return c
	}
	s := m.Solids
	for _, c := range []struct {
		what      string
		got, want box2d.Vec2
	}{
		{"the box's centre", centre(s[0].Polygon), at(-6, 0)},
		// (16, 32) px from its top left, turned 30° clockwise: (-2.14, 35.71) px.
		{"the turned box's centre", centre(s[1].Polygon), at((448-2.1436-320)/32.0, (192-64-35.7128)/32.0)},
		{"the circle's centre", s[3].Circle.Center.Vec2(), at(0, 3)},
		{"the capsule's first centre", s[4].Capsule.Center1.Vec2(), at(-1, -3)},
		{"the capsule's second centre", s[4].Capsule.Center2.Vec2(), at(1, -3)},
		{"the hidden turned capsule's first centre", s[5].Capsule.Center1.Vec2(), at(-4.5, 4)},
		{"the hidden turned capsule's second centre", s[5].Capsule.Center2.Vec2(), at(-4.5, 2)},
		{"the tall capsule's first centre", s[6].Capsule.Center1.Vec2(), at(8.5, 4.5)},
		{"the tall capsule's second centre", s[6].Capsule.Center2.Vec2(), at(8.5, 2.5)},
		{"the first spawn", m.Objects[0].At.Vec2(), at(-8, 0)},
		{"the second spawn", m.Objects[1].At.Vec2(), at(8, 0)},
	} {
		if !near(c.got, c.want) {
			t.Errorf("%s is at %v, want %v", c.what, c.got, c.want)
		}
	}
	if !slices.ContainsFunc(s[2].Polygon, func(p level.Point) bool { return near(p.Vec2(), at(6, -2.5)) }) {
		t.Errorf("the triangle %v has no tip at (6, -2.5)", s[2].Polygon)
	}
	if r := []int32{s[3].Circle.Radius, s[4].Capsule.Radius}; r[0] != 768 || r[1] != 512 {
		t.Errorf("radii %v units, want 0.75 and 0.5 m", r)
	}
	if teams := []any{m.Objects[0].Props, m.Objects[1].Props}; !reflect.DeepEqual(teams,
		[]any{spawn{Team: 0}, spawn{Team: 1}}) {
		t.Errorf("spawns' props %v, want team 0 (Tiled's default, unsaved) and 1", teams)
	}
}

// edit is a change to the yard's Tiled JSON.
type edit func(m map[string]any)

// object is the yard object with id.
func object(m map[string]any, id float64) map[string]any {
	for _, layer := range m["layers"].([]any) {
		for _, o := range layer.(map[string]any)["objects"].([]any) {
			if o := o.(map[string]any); o["id"] == id {
				return o
			}
		}
	}
	panic(fmt.Sprintf("the yard has no object %v", id))
}

// set sets object id's keys.
func set(id float64, keys map[string]any) edit {
	return func(m map[string]any) { maps.Copy(object(m, id), keys) }
}

func points(xy ...float64) []any {
	var out []any
	for i := 0; i < len(xy); i += 2 {
		out = append(out, map[string]any{"x": xy[i], "y": xy[i+1]})
	}
	return out
}

func team(typ string, value any) []any {
	return []any{map[string]any{"name": "team", "type": typ, "value": value}}
}

// Each Tiled rule refuses the yard with an error naming what broke it, and the object, its id and
// where it is.
func TestBakeRefuses(t *testing.T) {
	var nine []float64
	for i := range 9 {
		a := 2 * math.Pi * float64(i) / 9
		nine = append(nine, 32*math.Cos(a), 32*math.Sin(a))
	}
	for _, c := range []struct {
		name string
		edit edit
		want string
	}{
		{"an older Tiled", func(m map[string]any) { m["tiledversion"] = "1.11.2" }, `Tiled "1.11.2", want 1.12`},
		{"an infinite map", func(m map[string]any) { m["infinite"] = true }, "an infinite map"},
		{"an isometric map", func(m map[string]any) { m["orientation"] = "isometric" }, "want orthogonal"},
		{"tiles not square", func(m map[string]any) { m["tileheight"] = 16 }, "tiles 32 x 16 px, want square"},
		{"a tileset", func(m map[string]any) { m["tilesets"] = []any{map[string]any{"firstgid": 1}} }, "tilesets"},
		{"a tile layer", func(m map[string]any) {
			m["layers"] = append(m["layers"].([]any), map[string]any{"type": "tilelayer", "name": "ground"})
		}, `layer "ground" is a tilelayer`},
		{"two objects with one id", set(3, map[string]any{"id": 2.0}), "object 2 (wall) at (4, 4): its id"},
		{"a template", set(2, map[string]any{"template": "wall.tx"}), "template"},
		{"a tile", set(2, map[string]any{"gid": 5}), "is a tile"},
		{"text", set(2, map[string]any{"text": map[string]any{"text": "hi"}}), "is text"},
		{"no class", set(2, map[string]any{"type": ""}), "has no class"},
		{"a class the game lacks", set(2, map[string]any{"type": "lava"}),
			`bake: object 2 (lava) at (-6.5, 1): class "lava" is no kind or object type`},
		{"an ellipse", set(5, map[string]any{"width": 64}), "ellipse 64 x 48 px: a wall is a circle"},
		{"a square capsule", set(6, map[string]any{"width": 32}), "as long as it's wide"},
		{"a concave wall", set(4, map[string]any{"polygon": points(0, 0, 64, 0, 32, -48, 32, -8)}), "is concave"},
		{"a wall on a line", set(4, map[string]any{"polygon": points(0, 0, 32, 0, 64, 0, 32, -48)}), "on a line"},
		{"nine corners", set(4, map[string]any{"polygon": points(nine...)}), "9 corners, more than Box2D's 8"},
		{"a polyline", set(4, map[string]any{"polygon": nil, "polyline": points(0, 0, 64, 0)}), "polyline"},
		{"a point wall", set(2, map[string]any{"point": true}), "a wall is a shape"},
		{"a wall's property", set(2, map[string]any{"properties": team("int", 1)}), "a wall has no properties"},
		{"a round edge", set(1, map[string]any{"ellipse": true}), "a bounds is a rectangle or a polygon"},
		{"an edge crossing itself", set(1, map[string]any{"polygon": points(0, 0, 640, 384, 640, 0, 0, 384)}),
			"crosses itself"},
		{"an edge folding back", set(1, map[string]any{"polygon": points(0, 0, 640, 0, 320, 0, 320, 384)}),
			"crosses itself"},
		{"a spawn as a box", set(8, map[string]any{"point": false, "width": 32, "height": 32}), "a spawn is a point"},
		{"a property the spawn lacks", set(9, map[string]any{
			"properties": []any{map[string]any{"name": "side", "type": "int", "value": 1}},
		}), `has property "side"`},
		{"a property of another type", set(9, map[string]any{"properties": team("string", "1")}),
			`"team" is a Tiled string, want a int`},
		{"a property out of range", set(9, map[string]any{"properties": team("int", 300)}), `property "team"`},
		{"outside Box2D's world", set(2, map[string]any{"x": 1e9}), "outside Box2D's world"},
		{"a capsule too short for Box2D", set(6, map[string]any{"width": 32.1}),
			"yard: level: solids[4] (src 6): capsule's centers are LinearSlop"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal(yard(t), &m); err != nil {
				t.Fatal(err)
			}
			c.edit(m)
			tmj, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := bake.Bake("yard", tmj, config); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want an error with %q", err, c.want)
			}
		})
	}
}

// An edge drawn either way round, from any corner, bakes clockwise from its lowest, then leftmost.
func TestBakeWindsAnEdgeClockwiseFromItsLowestCorner(t *testing.T) {
	want, err := bake.Bake("yard", yard(t), config)
	if err != nil {
		t.Fatal(err)
	}
	for _, corners := range [][]float64{
		{640, 0, 640, 384, 0, 384, 0, 0}, // clockwise on screen, from the top right
		{0, 384, 640, 384, 640, 0, 0, 0}, // counter-clockwise on screen, from the bottom left
	} {
		var m map[string]any
		if err := json.Unmarshal(yard(t), &m); err != nil {
			t.Fatal(err)
		}
		set(1, map[string]any{"polygon": points(corners...)})(m)
		tmj, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := bake.Bake("yard", tmj, config); err != nil || string(got) != string(want) {
			t.Errorf("the edge drawn through %v baked %s (%v), want the rectangle's file", corners, got, err)
		}
	}
}

// The game's rules run on the map as a shard builds it, and a refusal names the map.
func TestBakeRunsTheGamesRules(t *testing.T) {
	c := config
	c.Check = func(m level.Map, world *box2d.World) error {
		shapes := 0
		everywhere := box2d.AABB{LowerBound: at(-100, -100), UpperBound: at(100, 100)}
		world.OverlapAABB(everywhere, box2d.DefaultQueryFilter(), func(box2d.ShapeID, any) bool {
			shapes++
			return true
		}, nil)
		return fmt.Errorf("a rule meets %d shapes and %d spawns", shapes, len(level.Objects[spawn](m)))
	}
	_, err := bake.Bake("yard", yard(t), c)
	if want := "yard: a rule meets 11 shapes and 2 spawns"; err == nil || err.Error() != want {
		t.Errorf("got %v, want %q: seven walls and the edge's four sides", err, want)
	}
}

// A config whose edge isn't a kind, or with a name both a kind and an object type, bakes nothing.
func TestBakeRefusesABadConfig(t *testing.T) {
	c := config
	c.Edges = []string{"fence"}
	c.Schema.Types = level.Types{"spawn": reflect.TypeFor[spawn](), "wall": reflect.TypeFor[spawn]()}
	_, err := bake.Bake("yard", yard(t), c)
	for _, want := range []string{`edge kind "fence"`, `"wall" is both a kind and an object type`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("got %v, want an error with %q", err, want)
		}
	}
	if _, err := bake.Project(c); err == nil {
		t.Error("Project wrote a project for it")
	}
}
