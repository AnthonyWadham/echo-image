package level_test

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
)

var kinds = level.Kinds{
	"wall":   {CategoryBits: 1, MaskBits: math.MaxUint64},
	"fill":   {CategoryBits: 2, MaskBits: math.MaxUint64},
	"bounds": {CategoryBits: 4, MaskBits: math.MaxUint64},
}

func newWorld() *box2d.World {
	def := box2d.DefaultWorldDef()
	return box2d.NewWorld(&def)
}

func meters(units int32) float64 { return float64(units) / level.Unit }

// Build makes each shape as the file has it, in file order: its user data is its place, its filter its
// kind's, and its geometry the file's, in meters.
func TestBuildMakesEachShapeInFileOrder(t *testing.T) {
	world, m := newWorld(), arena()
	body, err := level.Build(world, m, kinds)
	if err != nil {
		t.Fatal(err)
	}
	shapes := make([]box2d.ShapeID, world.BodyShapeCount(body))
	world.BodyShapes(body, shapes)
	at := map[uint64]box2d.ShapeID{}
	for _, shape := range shapes {
		at[world.ShapeUserData(shape)] = shape
	}
	var place uint64
	for _, s := range m.Solids {
		place++
		shape, ok := at[place]
		if !ok || world.ShapeFilter(shape) != kinds[s.Kind] {
			t.Fatalf("solid %d: no shape, or not kind %s's filter", place, s.Kind)
		}
		switch {
		case s.Circle != nil:
			want := box2d.Circle{Center: s.Circle.Center.Vec2(), Radius: meters(s.Circle.Radius)}
			if got := world.ShapeCircle(shape); got != want {
				t.Errorf("solid %d: circle %+v, want %+v", place, got, want)
			}
		case s.Capsule != nil:
			c := s.Capsule
			want := box2d.Capsule{Center1: c.Center1.Vec2(), Center2: c.Center2.Vec2(), Radius: meters(c.Radius)}
			if got := world.ShapeCapsule(shape); got != want {
				t.Errorf("solid %d: capsule %+v, want %+v", place, got, want)
			}
		default:
			got := world.ShapePolygon(shape)
			if got.Count != len(s.Polygon) || !slices.Equal(got.Vertices[:got.Count], vecs(s.Polygon)) {
				t.Errorf("solid %d: polygon %v, want %v", place, got.Vertices[:got.Count], vecs(s.Polygon))
			}
		}
	}
	for _, l := range slices.Concat(m.Outlines, m.Edges) {
		for i, p := range l.Points {
			place++
			shape, ok := at[place]
			if !ok || world.ShapeType(shape) != box2d.ChainSegmentShape || world.ShapeFilter(shape) != kinds[l.Kind] {
				t.Fatalf("segment %d: no chain segment, or not kind %s's filter", place, l.Kind)
			}
			seg := world.ShapeChainSegment(shape).Segment
			if next := l.Points[(i+1)%len(l.Points)]; seg.Point1 != p.Vec2() || seg.Point2 != next.Vec2() {
				t.Errorf("segment %d: %v to %v, want %v to %v", place, seg.Point1, seg.Point2, p.Vec2(), next.Vec2())
			}
		}
	}
	if len(shapes) != int(place) {
		t.Errorf("%d shapes, want %d", len(shapes), place)
	}
}

// Build adds nothing for a map it can't build as written: a kind the game didn't name, or a bad shape.
func TestBuildRefuses(t *testing.T) {
	for _, c := range []struct {
		name  string
		edit  func(*level.Map)
		kinds level.Kinds
		want  string
	}{
		{"an unknown kind", nil, level.Kinds{"wall": kinds["wall"], "bounds": kinds["bounds"]}, `unknown kind "fill"`},
		{"a bad shape", func(m *level.Map) { m.Edges[0].Points = m.Edges[0].Points[:3] }, kinds, "want 4 or more"},
	} {
		world, m := newWorld(), arena()
		if c.edit != nil {
			c.edit(&m)
		}
		if _, err := level.Build(world, m, c.kinds); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
		if n := world.Counters().BodyCount; n != 0 {
			t.Errorf("%s: it added %d bodies", c.name, n)
		}
	}
}
