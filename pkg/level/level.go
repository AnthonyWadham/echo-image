// Package level reads a baked map file and builds its walls into a Box2D world. A map is content, not
// state: the same bytes build the same walls on every platform, so a server and its clients agree
// without sending geometry. It imports only Box2D and the standard library, and the game names the
// kinds, so any game can use it.
package level

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
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

// A Map is a map file's shapes, each list in file order.
type Map struct {
	Solids   []Solid
	Outlines []Loop // loops that keep things out, around walls
	Edges    []Loop // loops that keep things in, around the play area
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
// meaning, and only shapes Box2D builds exactly as written.
func Decode(data []byte) (Map, error) {
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
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return Map{}, fmt.Errorf("level: %w", err)
	}
	m := Map{Solids: file.Solids, Outlines: file.Outlines, Edges: file.Edges}
	if err := m.check(); err != nil {
		return Map{}, err
	}
	// The decoder takes duplicate keys, keys in any case, -0, null and extra array items.
	if !bytes.Equal(encode(m), data) {
		return Map{}, errors.New("level: not in canonical form: write the file with Encode")
	}
	return m, nil
}

// Encode writes m as a map file, in the one form Decode reads. It refuses what Decode would.
func Encode(m Map) ([]byte, error) {
	if err := m.check(); err != nil {
		return nil, err
	}
	return encode(m), nil
}

// encode writes m one shape per line, keys in a fixed order. It writes what's there, checked or not.
func encode(m Map) []byte {
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
	b.WriteString("\n}\n")
	return b.Bytes()
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

// check returns the first shape Box2D can't build exactly as written.
func (m Map) check() error {
	for i, s := range m.Solids {
		if err := s.check(); err != nil {
			return fmt.Errorf("level: solids[%d] (src %d): %w", i, s.Src, err)
		}
	}
	for _, section := range m.sections() {
		for i, l := range section.loops {
			if err := l.check(section.winding); err != nil {
				return fmt.Errorf("level: %s[%d] (src %d): %w", section.name, i, l.Src, err)
			}
		}
	}
	return nil
}

func (s Solid) check() error {
	if err := checkKind(s.Kind); err != nil {
		return err
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
func (l Loop) check(want int) error {
	if err := checkKind(l.Kind); err != nil {
		return err
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
	switch w := winding(l.Points); {
	case w == 0:
		return errors.New("loop has no area")
	case w != want:
		return errors.New("loop winds the wrong way: an outline winds counter-clockwise, an edge clockwise")
	}
	return nil
}

// winding returns the sign of the loop's area, exactly: 1 counter-clockwise, -1 clockwise, 0 none.
func winding(points []Point) int {
	var sum, term big.Int
	for i, p := range points {
		q := points[(i+1)%len(points)]
		sum.Add(&sum, term.SetInt64(int64(p[0])*int64(q[1])-int64(q[0])*int64(p[1])))
	}
	return sum.Sign()
}

// checkKind takes 1 to 32 lowercase letters, digits and underscores, starting with a letter.
func checkKind(kind string) error {
	ok := len(kind) >= 1 && len(kind) <= 32 && kind[0] >= 'a' && kind[0] <= 'z'
	for _, r := range kind {
		ok = ok && (r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	}
	if !ok {
		return fmt.Errorf("kind %q: want a-z, 0-9 and _, starting with a letter, up to 32", kind)
	}
	return nil
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
