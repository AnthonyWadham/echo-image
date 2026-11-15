package level

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/goccy/go-json"
)

// Unit is how many file units make a meter. A whole number of units is an exact float64 in meters.
const Unit = 1024

// format is the file format Decode reads and Encode writes.
const format = 1

// A Point is x and y in units, y up.
type Point [2]int32

// Vec2 returns p in meters.
func (p Point) Vec2() box2d.Vec2 { return box2d.Vec2{X: meters(p[0]), Y: meters(p[1])} }

// A Map is a map file's contents, each list in file order.
type Map struct {
	Solids   []Solid
	Outlines []Loop // loops that keep things out, around walls
	Edges    []Loop // loops that keep things in, around the play area
	Objects  []Object
}

// A Solid is one convex piece: exactly one of a polygon, a circle and a capsule. Src is the drawn
// object it came from, and Kind the name the game gives its filter.
type Solid struct {
	Src     uint32   `json:"src"`
	Kind    string   `json:"kind"`
	Polygon []Point  `json:"polygon"` // 3 to 8 points, in the order box2d.ComputeHull returns them
	Circle  *Circle  `json:"circle"`
	Capsule *Capsule `json:"capsule"`
}

// A Circle is a center and a radius, in units.
type Circle struct {
	Center Point `json:"center"`
	Radius int32 `json:"radius"`
}

// A Capsule is two centers more than LinearSlop apart and a radius, in units.
type Capsule struct {
	Center1 Point `json:"center1"`
	Center2 Point `json:"center2"`
	Radius  int32 `json:"radius"`
}

// A Loop is a closed Box2D chain of 4 or more points. A chain blocks from the right of each segment,
// walking its points in order, so an outline winds counter-clockwise and an edge clockwise.
type Loop struct {
	Src    uint32  `json:"src"`
	Kind   string  `json:"kind"`
	Points []Point `json:"points"`
}

// An Object is a typed point in the map, such as a spawn. ID is unique in the map, from 1, but a
// rebake can change it. Props is a value of the struct the schema gives Type, not a pointer to one.
type Object struct {
	ID    uint32
	Type  string
	At    Point
	Props any
}

// A section is one of a map's lists of loops, and the way its loops wind: 1 counter-clockwise, -1
// clockwise.
type section struct {
	name    string
	loops   []Loop
	winding int
}

// sections returns m's lists of loops, in file order.
func (m Map) sections() []section {
	return []section{{"outlines", m.Outlines, 1}, {"edges", m.Edges, -1}}
}

// Decode reads a map file. It takes only the bytes Encode writes for the same map, so a file has one
// meaning, only kinds and types the schema names, and only shapes Box2D builds exactly as written.
func Decode(data []byte, schema Schema) (Map, error) {
	if err := schema.check(); err != nil {
		return Map{}, err
	}
	var head struct {
		Format int `json:"format"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return Map{}, fmt.Errorf("level: %w", err)
	}
	if head.Format != format {
		return Map{}, fmt.Errorf("level: format %d, want %d", head.Format, format)
	}
	var file struct {
		Format   int     `json:"format"`
		Solids   []Solid `json:"solids"`
		Outlines []Loop  `json:"outlines"`
		Edges    []Loop  `json:"edges"`
		Objects  []struct {
			ID    uint32          `json:"id"`
			Type  string          `json:"type"`
			At    Point           `json:"at"`
			Props json.RawMessage `json:"props"`
		} `json:"objects"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return Map{}, fmt.Errorf("level: %w", err)
	}
	m := Map{Solids: file.Solids, Outlines: file.Outlines, Edges: file.Edges}
	for i, o := range file.Objects {
		t, ok := schema.Types[o.Type]
		if !ok {
			return Map{}, fmt.Errorf("level: objects[%d] (id %d): unknown type %q", i, o.ID, o.Type)
		}
		props := reflect.New(t)
		propsDecoder := json.NewDecoder(bytes.NewReader(o.Props))
		propsDecoder.DisallowUnknownFields()
		if err := propsDecoder.Decode(props.Interface()); err != nil {
			return Map{}, fmt.Errorf("level: objects[%d] (id %d): props: %w", i, o.ID, err)
		}
		m.Objects = append(m.Objects, Object{ID: o.ID, Type: o.Type, At: o.At, Props: props.Elem().Interface()})
	}
	if err := m.check(schema); err != nil {
		return Map{}, err
	}
	// The decoder takes duplicate keys, keys in any case, -0, null, missing fields and extra array items.
	if canonical, err := encode(m); err != nil || !bytes.Equal(canonical, data) {
		return Map{}, errors.New("level: not in canonical form: write the file with Encode")
	}
	return m, nil
}

// Encode writes m as a map file, in the one form Decode reads. It refuses what Decode would.
func Encode(m Map, schema Schema) ([]byte, error) {
	if err := m.check(schema); err != nil {
		return nil, err
	}
	return encode(m)
}

// encode writes m one shape or object per line, keys in a fixed order, props as the JSON encoder writes
// them. It writes what's there, checked or not.
func encode(m Map) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "{\n  \"format\": %d,\n  \"solids\": [", format)
	for i, s := range m.Solids {
		item(&b, i)
		fmt.Fprintf(&b, `{"src": %d, "kind": "%s"`, s.Src, s.Kind)
		if s.Polygon != nil {
			fmt.Fprintf(&b, `, "polygon": %s`, list(s.Polygon))
		}
		if c := s.Circle; c != nil {
			fmt.Fprintf(&b, `, "circle": {"center": %s, "radius": %d}`, point(c.Center), c.Radius)
		}
		if c := s.Capsule; c != nil {
			fmt.Fprintf(&b, `, "capsule": {"center1": %s, "center2": %s, "radius": %d}`,
				point(c.Center1), point(c.Center2), c.Radius)
		}
		b.WriteByte('}')
	}
	end(&b, len(m.Solids))
	for _, section := range m.sections() {
		fmt.Fprintf(&b, ",\n  %q: [", section.name)
		for i, l := range section.loops {
			item(&b, i)
			fmt.Fprintf(&b, `{"src": %d, "kind": "%s", "points": %s}`, l.Src, l.Kind, list(l.Points))
		}
		end(&b, len(section.loops))
	}
	b.WriteString(",\n  \"objects\": [")
	for i, o := range m.Objects {
		props, err := json.Marshal(o.Props)
		if err != nil {
			return nil, fmt.Errorf("level: objects[%d] (id %d): props: %w", i, o.ID, err)
		}
		item(&b, i)
		fmt.Fprintf(&b, `{"id": %d, "type": "%s", "at": %s, "props": %s}`, o.ID, o.Type, point(o.At), props)
	}
	end(&b, len(m.Objects))
	b.WriteString("\n}\n")
	return b.Bytes(), nil
}

func item(b *bytes.Buffer, i int) {
	if i > 0 {
		b.WriteByte(',')
	}
	b.WriteString("\n    ")
}

func end(b *bytes.Buffer, n int) {
	if n > 0 {
		b.WriteString("\n  ")
	}
	b.WriteByte(']')
}

func point(p Point) string { return fmt.Sprintf("[%d, %d]", p[0], p[1]) }

func list(points []Point) string {
	s := make([]string, len(points))
	for i, p := range points {
		s[i] = point(p)
	}
	return "[" + strings.Join(s, ", ") + "]"
}

// check returns the first thing in m the schema doesn't name, or that Box2D can't build as written.
func (m Map) check(schema Schema) error {
	if err := schema.check(); err != nil {
		return err
	}
	for i, s := range m.Solids {
		if err := s.check(schema.Kinds); err != nil {
			return fmt.Errorf("level: solids[%d] (src %d): %w", i, s.Src, err)
		}
	}
	for _, section := range m.sections() {
		for i, l := range section.loops {
			if err := l.check(section.winding, schema.Kinds); err != nil {
				return fmt.Errorf("level: %s[%d] (src %d): %w", section.name, i, l.Src, err)
			}
		}
	}
	taken := map[uint32]bool{}
	for i, o := range m.Objects {
		if err := o.check(schema.Types, taken); err != nil {
			return fmt.Errorf("level: objects[%d] (id %d): %w", i, o.ID, err)
		}
	}
	return nil
}

func (s Solid) check(kinds Kinds) error {
	if _, ok := kinds[s.Kind]; !ok {
		return fmt.Errorf("unknown kind %q", s.Kind)
	}
	shapes := 0
	for _, has := range []bool{s.Polygon != nil, s.Circle != nil, s.Capsule != nil} {
		if has {
			shapes++
		}
	}
	if shapes != 1 {
		return fmt.Errorf("%d shapes, want one polygon, circle or capsule", shapes)
	}
	switch {
	case s.Circle != nil:
		return checkRound(s.Circle.Radius, s.Circle.Center)
	case s.Capsule != nil:
		c := s.Capsule
		if err := checkRound(c.Radius, c.Center1, c.Center2); err != nil {
			return err
		}
		// Box2D's own test: it builds no capsule this short.
		if box2d.DistanceSquared(c.Center1.Vec2(), c.Center2.Vec2()) <= box2d.LinearSlop*box2d.LinearSlop {
			return errors.New("capsule's centers are LinearSlop (5 mm) apart or less: write a circle")
		}
		return nil
	}
	return checkPolygon(s.Polygon)
}

func checkPolygon(points []Point) error {
	if n := len(points); n < 3 || n > box2d.MaxPolygonVertices {
		return fmt.Errorf("polygon has %d points, want 3 to %d", n, box2d.MaxPolygonVertices)
	}
	for _, p := range points {
		if err := inRange(p[0], p[1]); err != nil {
			return err
		}
	}
	hull := box2d.ComputeHull(vecs(points))
	if hull.Count != len(points) || !box2d.ValidateHull(&hull) {
		return errors.New("polygon isn't convex, or has points within LinearSlop (5 mm) or on a line")
	}
	for i, p := range points {
		if hull.Points[i] != p.Vec2() {
			return errors.New("polygon's points aren't in box2d.ComputeHull's order")
		}
	}
	return nil
}

// checkRound checks a radius of at least one unit, and that it and the centers are in range.
func checkRound(radius int32, centers ...Point) error {
	if radius < 1 {
		return fmt.Errorf("radius %d, want 1 or more", radius)
	}
	values := []int32{radius}
	for _, c := range centers {
		values = append(values, c[0], c[1])
	}
	return inRange(values...)
}

// check checks l as a loop that winds want: 1 counter-clockwise, -1 clockwise.
func (l Loop) check(want int, kinds Kinds) error {
	if _, ok := kinds[l.Kind]; !ok {
		return fmt.Errorf("unknown kind %q", l.Kind)
	}
	n := len(l.Points)
	if n < 4 {
		return fmt.Errorf("loop has %d points, want 4 or more", n)
	}
	for i, p := range l.Points {
		if err := inRange(p[0], p[1]); err != nil {
			return err
		}
		if box2d.DistanceSquared(p.Vec2(), l.Points[(i+1)%n].Vec2()) <= box2d.LinearSlop*box2d.LinearSlop {
			return fmt.Errorf("segment %d is LinearSlop (5 mm) or shorter", i)
		}
	}
	// Box2D's chains mustn't cross themselves, and it doesn't check.
	points := vecs(l.Points)
	for i := range n {
		for j := i + 2; j < n && (i > 0 || j < n-1); j++ { // segments that share no point
			d := box2d.SegmentDistance(points[i], points[(i+1)%n], points[j], points[(j+1)%n])
			if d.DistanceSquared <= box2d.LinearSlop*box2d.LinearSlop {
				return fmt.Errorf("segments %d and %d cross or come within LinearSlop (5 mm)", i, j)
			}
		}
	}
	switch w := Winding(l.Points); {
	case w == 0:
		return errors.New("loop has no area")
	case w != want:
		return errors.New("loop winds the wrong way: an outline winds counter-clockwise, an edge clockwise")
	}
	return nil
}

// Winding returns the sign of the loop's area: 1 counter-clockwise, -1 clockwise, 0 none. It's exact for
// a loop in Box2D's world that doesn't cross itself: twice its area fits an int64.
func Winding(points []Point) int {
	var sum int64
	for i, p := range points {
		q := points[(i+1)%len(points)]
		sum += int64(p[0])*int64(q[1]) - int64(q[0])*int64(p[1])
	}
	return cmp.Compare(sum, 0)
}

// check checks o's ID is from 1 and not taken, its type is in types with props of its struct, and it
// lies in range. It marks the ID taken.
func (o Object) check(types Types, taken map[uint32]bool) error {
	switch {
	case o.ID == 0:
		return errors.New("id 0, want 1 or more")
	case taken[o.ID]:
		return errors.New("an earlier object has this id")
	}
	taken[o.ID] = true
	t, ok := types[o.Type]
	if !ok {
		return fmt.Errorf("unknown type %q", o.Type)
	}
	if got := reflect.TypeOf(o.Props); got != t {
		return fmt.Errorf("props are a %v, want a %v", got, t)
	}
	return inRange(o.At[0], o.At[1])
}

// inRange checks each value, in units, is within Box2D's world: box2d.Huge meters.
func inRange(values ...int32) error {
	for _, v := range values {
		if math.Abs(meters(v)) > box2d.Huge {
			return fmt.Errorf("%d units is outside Box2D's world (%g m)", v, box2d.Huge)
		}
	}
	return nil
}

func meters(units int32) float64 { return float64(units) / Unit }

func vecs(points []Point) []box2d.Vec2 {
	v := make([]box2d.Vec2, len(points))
	for i, p := range points {
		v[i] = p.Vec2()
	}
	return v
}
