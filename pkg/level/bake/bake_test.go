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
			"fill":   {CategoryBits: 4, MaskBits: math.MaxUint64},
		},
		Types: level.Types{"spawn": reflect.TypeFor[spawn]()},
	},
	Edges: []string{"bounds"},
	Fills: map[string]string{"wall": "fill"},
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

func vecs(points []level.Point) []box2d.Vec2 {
	out := make([]box2d.Vec2, len(points))
	for i, p := range points {
		out[i] = p.Vec2()
	}
	return out
}

// The yard is a 20 x 12 m canvas of 32 px tiles: an edge around it, a wall of each shape, one turned 30°,
// a hidden capsule turned 90° and one drawn tall, an L, and a spawn each side. It bakes to the golden file, in
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
	// The L is its outline, counter-clockwise from its lowest left corner, and four pieces of fill.
	ell := []box2d.Vec2{at(-7, -5.5), at(-6, -5.5), at(-6, -4), at(-4, -4), at(-4, -3), at(-7, -3)}
	if len(m.Outlines) != 1 || m.Outlines[0].Kind != "wall" || !reflect.DeepEqual(vecs(m.Outlines[0].Points), ell) {
		t.Errorf("outlines %v, want the L's: %v", m.Outlines, ell)
	}
	for _, piece := range s[7:] {
		if piece.Kind != "fill" || piece.Src != 11 || len(piece.Polygon) != 3 {
			t.Errorf("the L's piece %+v, want a triangle of fill from object 11", piece)
		}
	}
	if len(s) != 11 {
		t.Errorf("%d solids, want the 7 walls and the L's 4 pieces", len(s))
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
	unfilled := config // a game whose walls have no fill kind
	unfilled.Fills = nil
	type refusal struct {
		name string
		edit edit
		want string
	}
	refuses := func(with bake.Config, cases ...refusal) {
		for _, c := range cases {
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
				if _, err := bake.Bake("yard", tmj, with); err == nil || !strings.Contains(err.Error(), c.want) {
					t.Errorf("got %v, want an error with %q", err, c.want)
				}
			})
		}
	}
	refuses(unfilled,
		refusal{"a concave wall", set(4, map[string]any{"polygon": points(0, 0, 64, 0, 32, -48, 32, -8)}), "is concave"},
		refusal{"a wall on a line", set(4, map[string]any{"polygon": points(0, 0, 32, 0, 64, 0, 32, -48)}), "on a line"},
		refusal{"nine corners", set(4, map[string]any{"polygon": points(nine...)}), "9 corners, more than Box2D's 8"},
		refusal{"a wall whose corners cross", set(4, map[string]any{"polygon": points(0, 0, 64, 0, 0, -48, 64, -48)}),
			"crosses itself"},
	)
	refuses(config, []refusal{
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
		{"a shape drawn as a fill", set(2, map[string]any{"type": "fill"}), `class "fill" is a fill, which the baker makes`},
		{"a class the game lacks", set(2, map[string]any{"type": "lava"}),
			`bake: object 2 (lava) at (-6.5, 1): class "lava" is no kind or object type`},
		{"an ellipse", set(5, map[string]any{"width": 64}), "ellipse 64 x 48 px: a wall is a circle"},
		{"a square capsule", set(6, map[string]any{"width": 32}), "as long as it's wide"},
		// A 20 m bar 0.25 m thick with a stub under its middle: every piece on the stub's top is a sliver.
		{"a wall too thin to cut", set(11, map[string]any{"polygon": points(0, 0, 640, 0, 640, 8, 324, 8, 324, 16,
			316, 16, 316, 8, 0, 8), "x": 0.0}), "object 11 (wall) at (-10, -3): can't be cut into pieces Box2D takes"},
		{"a wall that crosses itself", set(11, map[string]any{"polygon": points(0, 0, 96, 0, 96, 64, 48, -32, 0, 64)}),
			"object 11 (wall) at (-7, -3): level: outlines[0] (src 11): segments"},
		{"a polyline", set(4, map[string]any{"polygon": nil, "polyline": points(0, 0, 64, 0)}), "polyline"},
		{"a point wall", set(2, map[string]any{"point": true}), "a wall is a shape"},
		{"a wall's property", set(2, map[string]any{"properties": team("int", 1)}), "a wall has no properties"},
		{"a round edge", set(1, map[string]any{"ellipse": true}), "a bounds is a rectangle or a polygon"},
		{"an edge crossing itself", set(1, map[string]any{"polygon": points(0, 0, 640, 384, 640, 0, 0, 384)}),
			"edges[0] (src 1): segments"},
		{"an edge folding back", set(1, map[string]any{"polygon": points(0, 0, 640, 0, 320, 0, 320, 384)}),
			"edges[0] (src 1): segments"},
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
	}...)
}

// A wall Box2D takes as no one polygon (concave, more than 8 corners, a corner on a line) bakes as its
// outline, counter-clockwise from its lowest left corner however it was drawn, and triangles of its fill
// kind that cover its inside and nothing else. Each shape replaces the yard's L, at (-7, -3).
func TestBakeCutsAWallIntoItsOutlineAndFill(t *testing.T) {
	var round []float64
	for i := range 12 {
		a := 2 * math.Pi * float64(i) / 12
		round = append(round, 48+48*math.Cos(a), 48+48*math.Sin(a))
	}
	for _, c := range []struct {
		name   string
		px     []float64 // its corners in pixels, y down, as Tiled draws them
		turned float64   // degrees clockwise
	}{
		{"an L", []float64{0, 0, 96, 0, 96, 32, 32, 32, 32, 80, 0, 80}, 0},
		{"an L drawn the other way round", []float64{32, 32, 96, 32, 96, 0, 0, 0, 0, 80, 32, 80}, 0},
		{"an L turned 30°", []float64{0, 0, 96, 0, 96, 32, 32, 32, 32, 80, 0, 80}, 30},
		{"a U", []float64{0, 0, 32, 0, 32, 64, 64, 64, 64, 0, 96, 0, 96, 96, 0, 96}, 0},
		{"a T", []float64{0, 0, 96, 0, 96, 32, 64, 32, 64, 96, 32, 96, 32, 32, 0, 32}, 0},
		{"a plus", []float64{32, 0, 64, 0, 64, 32, 96, 32, 96, 64, 64, 64, 64, 96, 32, 96, 32, 64, 0, 64, 0, 32, 32, 32}, 0},
		{"a round wall of 12 corners", round, 0},
		{"a box with a corner on a line", []float64{0, 0, 48, 0, 96, 0, 96, 64, 0, 64}, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			var tiled map[string]any
			if err := json.Unmarshal(yard(t), &tiled); err != nil {
				t.Fatal(err)
			}
			set(11, map[string]any{"polygon": points(c.px...), "rotation": c.turned})(tiled)
			tmj, err := json.Marshal(tiled)
			if err != nil {
				t.Fatal(err)
			}
			data, err := bake.Bake("yard", tmj, config)
			if err != nil {
				t.Fatal(err)
			}
			m, err := level.Decode(data, config.Schema)
			if err != nil {
				t.Fatal(err)
			}
			// Where Tiled draws it, in meters: turned about its origin at (96, 288) px, then y up.
			sin, cos := math.Sincos(c.turned * math.Pi / 180)
			var drawn []box2d.Vec2
			for i := 0; i < len(c.px); i += 2 {
				x, y := c.px[i]*cos-c.px[i+1]*sin, c.px[i]*sin+c.px[i+1]*cos
				drawn = append(drawn, at((96+x-320)/32, (192-288-y)/32))
			}
			n := len(drawn)
			if len(m.Outlines) != 1 || len(m.Outlines[0].Points) != n || len(m.Solids) != 7+n-2 {
				t.Fatalf("%d outlines and %d solids, want one outline of %d corners and %d pieces after the 7 walls",
					len(m.Outlines), len(m.Solids), n, n-2)
			}
			outline := m.Outlines[0].Points
			for i, p := range outline {
				if p[1] < outline[0][1] || p[1] == outline[0][1] && p[0] < outline[0][0] {
					t.Errorf("the outline starts at %v, not its lowest left corner: corner %d is %v", outline[0], i, p)
				}
			}
			def := box2d.DefaultWorldDef()
			world := box2d.NewWorld(&def)
			if _, err := level.Build(world, m, config.Schema); err != nil {
				t.Fatal(err)
			}
			fill := box2d.QueryFilter{CategoryBits: 1, MaskBits: config.Schema.Kinds["fill"].CategoryBits}
			low, high := drawn[0], drawn[0]
			for _, p := range drawn {
				low, high = at(min(low.X, p.X), min(low.Y, p.Y)), at(max(high.X, p.X), max(high.Y, p.Y))
			}
			for x := low.X - 0.25; x <= high.X+0.25; x += 1.0 / 16 {
				for y := low.Y - 0.25; y <= high.Y+0.25; y += 1.0 / 16 {
					inside, near := within(drawn, at(x, y))
					if near < 0.002 { // a corner rounds to the file's grid, 1 mm
						continue
					}
					proxy := box2d.MakeProxy([]box2d.Vec2{at(x, y)}, 1, 0)
					filled := false
					world.OverlapShape(&proxy, fill, func(box2d.ShapeID, any) bool {
						filled = true
						return false
					}, nil)
					if filled != inside {
						t.Fatalf("(%v, %v): in the fill %v, in the wall as drawn %v", x, y, filled, inside)
					}
				}
			}
		})
	}
}

// within reports whether p is inside the polygon, by how many of its sides a ray from p crosses, and
// how near p is to a side.
func within(polygon []box2d.Vec2, p box2d.Vec2) (inside bool, near float64) {
	near = math.Inf(1)
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < a.X+(b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y) {
			inside = !inside
		}
		near = min(near, math.Sqrt(box2d.SegmentDistance(a, b, p, p).DistanceSquared))
	}
	return inside, near
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
	if want := "yard: a rule meets 21 shapes and 2 spawns"; err == nil || err.Error() != want {
		t.Errorf("got %v, want %q: seven walls, the L's four pieces and six sides, the edge's four sides", err, want)
	}
}

// A config whose edge or fill isn't a kind, or with a name both a kind and an object type, bakes nothing.
func TestBakeRefusesABadConfig(t *testing.T) {
	c := config
	c.Edges = []string{"fence"}
	c.Fills = map[string]string{"wall": "glue"}
	c.Schema.Types = level.Types{"spawn": reflect.TypeFor[spawn](), "wall": reflect.TypeFor[spawn]()}
	_, err := bake.Bake("yard", yard(t), c)
	for _, want := range []string{`edge kind "fence"`, `fill of "wall": "glue" isn't`,
		`"wall" is both a kind and an object type`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("got %v, want an error with %q", err, want)
		}
	}
	if _, err := bake.Project(c); err == nil {
		t.Error("Project wrote a project for it")
	}
}
