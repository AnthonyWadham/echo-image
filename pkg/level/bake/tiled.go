package bake

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/goccy/go-json"

	"echo_image/pkg/level"
)

// tiledMap is the part of a Tiled map file the baker reads. Tiled writes more, for its editor.
type tiledMap struct {
	TiledVersion string            `json:"tiledversion"`
	Orientation  string            `json:"orientation"`
	Infinite     bool              `json:"infinite"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	TileWidth    int               `json:"tilewidth"`
	TileHeight   int               `json:"tileheight"`
	Tilesets     []json.RawMessage `json:"tilesets"`
	Layers       []tiledLayer      `json:"layers"`
}

type tiledLayer struct {
	Type    string        `json:"type"`
	Name    string        `json:"name"`
	Objects []tiledObject `json:"objects"`
}

// tiledObject is one drawn object, in pixels: x right and y down from the canvas's top left. A shape's
// points are from (X, Y), and it turns Rotation degrees clockwise about it.
type tiledObject struct {
	ID         uint32          `json:"id"`
	Class      string          `json:"type"` // Tiled 1.9 renamed type to class, but saves it as type
	X          float64         `json:"x"`
	Y          float64         `json:"y"`
	Width      float64         `json:"width"`
	Height     float64         `json:"height"`
	Rotation   float64         `json:"rotation"`
	Point      bool            `json:"point"`
	Ellipse    bool            `json:"ellipse"`
	Capsule    bool            `json:"capsule"`
	Polygon    []tiledPoint    `json:"polygon"`
	Polyline   []tiledPoint    `json:"polyline"`
	Text       json.RawMessage `json:"text"`
	GID        uint32          `json:"gid"`
	Template   string          `json:"template"`
	Properties []tiledProperty `json:"properties"`
}

type tiledPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type tiledProperty struct {
	Name  string          `json:"name"`
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// reader builds a level.Map from a Tiled map's objects.
type reader struct {
	c      Config
	canvas canvas
	m      level.Map
	ids    map[uint32]bool
}

// read reads a Tiled map into a level.Map: every error it finds, each naming its object.
func read(tmj []byte, c Config) (level.Map, error) {
	var t tiledMap
	if err := json.Unmarshal(tmj, &t); err != nil {
		return level.Map{}, fmt.Errorf("bake: not a Tiled map: %w", err)
	}
	if err := t.check(); err != nil {
		return level.Map{}, err
	}
	r := reader{c: c, ids: map[uint32]bool{}, canvas: canvas{
		tile: float64(t.TileWidth),
		cx:   float64(t.Width*t.TileWidth) / 2,
		cy:   float64(t.Height*t.TileHeight) / 2,
	}}
	var errs []error
	for _, layer := range t.Layers {
		if layer.Type != "objectgroup" {
			errs = append(errs, fmt.Errorf("bake: layer %q is a %s: only object layers bake", layer.Name, layer.Type))
			continue
		}
		for _, o := range layer.Objects {
			if err := r.object(o); err != nil {
				errs = append(errs, fmt.Errorf("bake: object %d (%s) at %s: %w", o.ID, o.Class, r.canvas.at(o), err))
			}
		}
	}
	return r.m, errors.Join(errs...)
}

// check returns an error unless t is a finite orthogonal map of square tiles, with no tilesets, saved by
// Tiled 1.12 or later: earlier ones save a capsule as a rectangle.
func (t tiledMap) check() error {
	var errs []error
	if major, minor, ok := version(t.TiledVersion); !ok || major < 1 || major == 1 && minor < 12 {
		errs = append(errs, fmt.Errorf("bake: saved by Tiled %q, want 1.12 or later", t.TiledVersion))
	}
	if t.Orientation != "orthogonal" {
		errs = append(errs, fmt.Errorf("bake: a %s map, want orthogonal", t.Orientation))
	}
	if t.Infinite {
		errs = append(errs, errors.New("bake: an infinite map, want a fixed size"))
	}
	if t.TileWidth <= 0 || t.TileWidth != t.TileHeight {
		errs = append(errs, fmt.Errorf("bake: tiles %d x %d px, want square: a tile is a meter", t.TileWidth,
			t.TileHeight))
	}
	if len(t.Tilesets) > 0 {
		errs = append(errs, errors.New("bake: the map has tilesets: only shapes and points bake"))
	}
	return errors.Join(errs...)
}

// version reads a Tiled version such as "1.12.2".
func version(s string) (int, int, bool) {
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	return major, minor, err1 == nil && err2 == nil
}

// object adds o to the map: a shape of a kind, or a point of an object type.
func (r *reader) object(o tiledObject) error {
	switch {
	case o.ID == 0 || r.ids[o.ID]:
		return errors.New("its id is 0 or another object's: give each object its own")
	case o.Template != "":
		return errors.New("is a template instance, which leaves out fields: detach it")
	case o.GID != 0:
		return errors.New("is a tile: only shapes and points bake")
	case len(o.Text) > 0:
		return errors.New("is text: only shapes and points bake")
	case o.Class == "":
		return errors.New("has no class: give it a kind or an object type")
	case r.c.fills(o.Class):
		return fmt.Errorf("class %q is a fill, which the baker makes: draw the wall it fills", o.Class)
	}
	r.ids[o.ID] = true
	if _, ok := r.c.Schema.Kinds[o.Class]; ok {
		return r.shape(o)
	}
	if t, ok := r.c.Schema.Types[o.Class]; ok {
		return r.point(o, t)
	}
	return fmt.Errorf("class %q is no kind or object type of the game", o.Class)
}

// shape adds o, a shape of a kind: an edge loop, or a solid.
func (r *reader) shape(o tiledObject) error {
	edge := slices.Contains(r.c.Edges, o.Class)
	switch {
	case len(o.Properties) > 0:
		return fmt.Errorf("a %s has no properties", o.Class)
	case o.Point:
		return fmt.Errorf("is a point: a %s is a shape", o.Class)
	case o.Polyline != nil:
		return errors.New("is a polyline, which has no inside: close it into a polygon")
	case edge && (o.Ellipse || o.Capsule):
		return fmt.Errorf("a %s is a rectangle or a polygon", o.Class)
	case o.Ellipse:
		return r.circle(o)
	case o.Capsule:
		return r.capsule(o)
	}
	corners := []tiledPoint{{0, 0}, {o.Width, 0}, {o.Width, o.Height}, {0, o.Height}}
	if o.Polygon != nil {
		corners = o.Polygon
	}
	points := make([]level.Point, len(corners))
	for i, p := range corners {
		var err error
		if points[i], err = r.canvas.point(o, p.X, p.Y); err != nil {
			return err
		}
	}
	if edge {
		r.m.Edges = append(r.m.Edges, level.Loop{Src: o.ID, Kind: o.Class, Points: edgeLoop(points)})
		return nil
	}
	polygon, err := convex(points)
	if err != nil {
		return err
	}
	r.m.Solids = append(r.m.Solids, level.Solid{Src: o.ID, Kind: o.Class, Polygon: polygon})
	return nil
}

// circle adds o, an ellipse as wide as it's tall.
func (r *reader) circle(o tiledObject) error {
	if o.Width != o.Height {
		return fmt.Errorf("is an ellipse %g x %g px: a %s is a circle", o.Width, o.Height, o.Class)
	}
	center, err := r.canvas.point(o, o.Width/2, o.Height/2)
	if err != nil {
		return err
	}
	c := &level.Circle{Center: center, Radius: r.canvas.length(o.Width / 2)}
	r.m.Solids = append(r.m.Solids, level.Solid{Src: o.ID, Kind: o.Class, Circle: c})
	return nil
}

// capsule adds o: its radius half its short side, its centers on its long axis.
func (r *reader) capsule(o tiledObject) error {
	if o.Width == o.Height {
		return fmt.Errorf("is a capsule as long as it's wide: draw a circle")
	}
	radius := min(o.Width, o.Height) / 2
	ends := [2]tiledPoint{{radius, o.Height / 2}, {o.Width - radius, o.Height / 2}}
	if o.Height > o.Width {
		ends = [2]tiledPoint{{o.Width / 2, radius}, {o.Width / 2, o.Height - radius}}
	}
	var centers [2]level.Point
	for i, e := range ends {
		var err error
		if centers[i], err = r.canvas.point(o, e.X, e.Y); err != nil {
			return err
		}
	}
	c := &level.Capsule{Center1: centers[0], Center2: centers[1], Radius: r.canvas.length(radius)}
	r.m.Solids = append(r.m.Solids, level.Solid{Src: o.ID, Kind: o.Class, Capsule: c})
	return nil
}

// point adds o, a point of an object type, with its properties as t's props.
func (r *reader) point(o tiledObject, t reflect.Type) error {
	if !o.Point {
		return fmt.Errorf("a %s is a point", o.Class)
	}
	at, err := r.canvas.point(o, 0, 0)
	if err != nil {
		return err
	}
	props, err := propsOf(o.Properties, t)
	if err != nil {
		return err
	}
	r.m.Objects = append(r.m.Objects, level.Object{ID: o.ID, Type: o.Class, At: at, Props: props})
	return nil
}

// propsOf is a t holding properties: each one of t's fields, of its Tiled type. A field with none keeps
// its zero value, the default the project gives it.
func propsOf(properties []tiledProperty, t reflect.Type) (any, error) {
	fs, err := fields(t)
	if err != nil {
		return nil, err
	}
	v := reflect.New(t).Elem()
	for _, p := range properties {
		i := slices.IndexFunc(fs, func(f field) bool { return f.name == p.Name })
		switch {
		case i < 0:
			return nil, fmt.Errorf("has property %q, which a %v doesn't", p.Name, t)
		case p.Type != fs[i].tiled:
			return nil, fmt.Errorf("property %q is a Tiled %s, want a %s", p.Name, p.Type, fs[i].tiled)
		}
		if err := json.Unmarshal(p.Value, v.Field(fs[i].index).Addr().Interface()); err != nil {
			return nil, fmt.Errorf("property %q: %w", p.Name, err)
		}
	}
	return v.Interface(), nil
}

// convex is a wall's corners in box2d.ComputeHull's order, which a map file holds them in, or an error
// unless every corner is on the hull.
func convex(points []level.Point) ([]level.Point, error) {
	if n := len(points); n > box2d.MaxPolygonVertices {
		return nil, fmt.Errorf("has %d corners, more than Box2D's %d", n, box2d.MaxPolygonVertices)
	}
	vecs := make([]box2d.Vec2, len(points))
	for i, p := range points {
		vecs[i] = p.Vec2()
	}
	hull := box2d.ComputeHull(vecs)
	if hull.Count != len(points) {
		return nil, errors.New("is concave, or has a corner on a line or within 5 mm of another: " +
			"a wall is convex (concave walls come with the fill kind)")
	}
	out := make([]level.Point, hull.Count)
	for i, v := range hull.Points[:hull.Count] {
		out[i] = level.Point{int32(math.Round(v.X * level.Unit)), int32(math.Round(v.Y * level.Unit))}
	}
	return out, nil
}

// edgeLoop is an edge's corners clockwise from its lowest, then leftmost.
func edgeLoop(points []level.Point) []level.Point {
	loop := slices.Clone(points)
	if level.Winding(loop) > 0 { // counter-clockwise
		slices.Reverse(loop)
	}
	first := 0
	for i, p := range loop {
		if q := loop[first]; p[1] < q[1] || p[1] == q[1] && p[0] < q[0] {
			first = i
		}
	}
	return append(loop[first:], loop[:first]...)
}

// canvas turns Tiled's pixels, y down from the canvas's top left, into units, y up from its centre: a
// tile is a meter.
type canvas struct {
	tile   float64 // pixels a meter
	cx, cy float64 // the centre, in pixels
}

// point is the point (x, y) pixels from o's origin, turned with o, in units. Each product is its own
// float64, so no platform fuses them. math.Sincos can still differ in its last digit on arm64: a turned
// corner that rounds on a half unit could bake one unit off there.
func (c canvas) point(o tiledObject, x, y float64) (level.Point, error) {
	px, py := o.X+x, o.Y+y
	if o.Rotation != 0 {
		sin, cos := math.Sincos(o.Rotation * math.Pi / 180)
		px = o.X + float64(x*cos) - float64(y*sin)
		py = o.Y + float64(x*sin) + float64(y*cos)
	}
	ux, uy := c.units(px-c.cx), c.units(c.cy-py)
	if math.Abs(ux) > box2d.Huge*level.Unit || math.Abs(uy) > box2d.Huge*level.Unit {
		return level.Point{}, fmt.Errorf("is outside Box2D's world (%g m)", box2d.Huge)
	}
	return level.Point{int32(ux), int32(uy)}, nil
}

// units is px pixels in whole units.
func (c canvas) units(px float64) float64 { return math.Round(float64(px*level.Unit) / c.tile) }

// length is a length of px pixels in units.
func (c canvas) length(px float64) int32 { return int32(c.units(px)) }

// at is o's origin in meters, for an error.
func (c canvas) at(o tiledObject) string {
	return fmt.Sprintf("(%g, %g)", (o.X-c.cx)/c.tile, (c.cy-o.Y)/c.tile)
}
