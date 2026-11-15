package level

import (
	"errors"
	"fmt"
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// blockCells is the side, in cells, of a block one Box2D AABB query samples at once.
const blockCells = 8

// Grid samples a built map for a game's rules: square cells of side Cell, centred on whole multiples of
// it, over the map's shapes and objects and a ring of cells around them. A cell is open when a circle of
// the grid's radius at its centre touches no shape the filter accepts. Rules flood it to see where that
// circle can go. Like every sampled check, it can miss a gap narrower than a cell.
type Grid struct {
	cell float64
	lo   [2]int // the first cell's index on each axis
	w, h int
	open []bool
}

// NewGrid samples m, built into world, for a circle of radius: one Box2D AABB query per block of cells,
// then one circle overlap per cell of a block that query finds a shape near. The radius must be at least
// half a cell, so no shape fits between two open neighbours.
func NewGrid(world *box2d.World, m Map, cell, radius float64, filter box2d.QueryFilter) (*Grid, error) {
	if !(cell > 0) || radius < cell/2 {
		return nil, fmt.Errorf("level: a grid of %v m cells samples circles of radius %v m or more, not %v",
			cell, cell/2, radius)
	}
	lo, hi, ok := m.bounds()
	if !ok {
		return nil, errors.New("level: an empty map has nothing to sample")
	}
	margin := int(math.Ceil(radius/cell)) + 1 // so a ring cell's circle clears every shape
	g := &Grid{cell: cell}
	g.lo = [2]int{int(math.Floor(lo.X/cell)) - margin, int(math.Floor(lo.Y/cell)) - margin}
	g.w = int(math.Ceil(hi.X/cell)) + margin - g.lo[0] + 1
	g.h = int(math.Ceil(hi.Y/cell)) + margin - g.lo[1] + 1
	g.open = make([]bool, g.w*g.h)
	q := &probe{}
	for j := 0; j < g.h; j += blockCells {
		for i := 0; i < g.w; i += blockCells {
			g.sample(world, i, j, radius, filter, q)
		}
	}
	return g, nil
}

// probe is what every query of one grid reuses, so sampling allocates nothing per cell.
type probe struct {
	proxy box2d.ShapeProxy
	hit   bool
}

// sample samples the block of cells from (i, j).
func (g *Grid) sample(world *box2d.World, i, j int, radius float64, filter box2d.QueryFilter, q *probe) {
	ei, ej := min(i+blockCells, g.w), min(j+blockCells, g.h)
	first, last := g.Center(j*g.w+i), g.Center((ej-1)*g.w+ei-1)
	box := box2d.AABB{
		LowerBound: box2d.Vec2{X: first.X - radius, Y: first.Y - radius},
		UpperBound: box2d.Vec2{X: last.X + radius, Y: last.Y + radius},
	}
	q.hit = false
	world.OverlapAABB(box, filter, touch, &q.hit)
	if !q.hit {
		for y := j; y < ej; y++ {
			for x := i; x < ei; x++ {
				g.open[y*g.w+x] = true
			}
		}
		return
	}
	for y := j; y < ej; y++ {
		for x := i; x < ei; x++ {
			k := y*g.w + x
			q.proxy, q.hit = box2d.MakeProxy([]box2d.Vec2{g.Center(k)}, 1, radius), false
			world.OverlapShape(&q.proxy, filter, touch, &q.hit)
			g.open[k] = !q.hit
		}
	}
}

// touch notes a hit in its context, a *bool, and ends the query.
func touch(_ box2d.ShapeID, hit any) bool {
	if h, ok := hit.(*bool); ok {
		*h = true
	}
	return false
}

// Cell is the side of a cell, in meters.
func (g *Grid) Cell() float64 { return g.cell }

// Len is how many cells the grid has. Cells are numbered from 0, row by row from the lowest.
func (g *Grid) Len() int { return len(g.open) }

// Center is cell k's centre.
func (g *Grid) Center(k int) box2d.Vec2 {
	i, j := g.lo[0]+k%g.w, g.lo[1]+k/g.w
	return box2d.Vec2{X: float64(float64(i) * g.cell), Y: float64(float64(j) * g.cell)}
}

// Open reports whether a circle at cell k's centre touches nothing.
func (g *Grid) Open(k int) bool { return g.open[k] }

// Nearest returns the cell whose square holds p, or ok=false when p is off the grid.
func (g *Grid) Nearest(p box2d.Vec2) (int, bool) {
	i, j := int(math.Round(p.X/g.cell))-g.lo[0], int(math.Round(p.Y/g.cell))-g.lo[1]
	if i < 0 || i >= g.w || j < 0 || j >= g.h {
		return 0, false
	}
	return j*g.w + i, true
}

// Border returns the ring of cells around the grid's edge: open, and outside every shape of the map.
func (g *Grid) Border() []int {
	ring := make([]int, 0, 2*(g.w+g.h))
	for i := range g.w {
		ring = append(ring, i, (g.h-1)*g.w+i)
	}
	for j := 1; j < g.h-1; j++ {
		ring = append(ring, j*g.w, j*g.w+g.w-1)
	}
	return ring
}

// Flood returns which cells a circle reaches from the open cells of from, moving one cell at a time,
// along an axis, between open cells. It fills a row's run of open cells at once, then seeds each run it
// touches above and below.
func (g *Grid) Flood(from []int) []bool {
	reached := make([]bool, len(g.open))
	var seeds []int
	for _, k := range from {
		if g.open[k] {
			seeds = append(seeds, k)
		}
	}
	for len(seeds) > 0 {
		k := seeds[len(seeds)-1]
		seeds = seeds[:len(seeds)-1]
		if reached[k] {
			continue
		}
		lo, hi := g.run(k, reached)
		for x := lo; x <= hi; x++ {
			reached[x] = true
		}
		seeds = g.seed(seeds, lo-g.w, hi-g.w, reached)
		seeds = g.seed(seeds, lo+g.w, hi+g.w, reached)
	}
	return reached
}

// run is the first and last cell of the run of open, unreached cells along k's row through k.
func (g *Grid) run(k int, reached []bool) (int, int) {
	row := k - k%g.w
	lo, hi := k, k
	for lo > row && g.open[lo-1] && !reached[lo-1] {
		lo--
	}
	for hi < row+g.w-1 && g.open[hi+1] && !reached[hi+1] {
		hi++
	}
	return lo, hi
}

// seed appends the first cell of each run of open, unreached cells from lo to hi, a stretch of one row,
// unless that row is off the grid.
func (g *Grid) seed(seeds []int, lo, hi int, reached []bool) []int {
	if lo < 0 || hi >= len(g.open) {
		return seeds
	}
	run := false // in a run already seeded
	for x := lo; x <= hi; x++ {
		next := g.open[x] && !reached[x]
		if next && !run {
			seeds = append(seeds, x)
		}
		run = next
	}
	return seeds
}

// bounds is the box around m's shapes and objects, in meters, or ok=false when it has none.
func (m Map) bounds() (box2d.Vec2, box2d.Vec2, bool) {
	var units []Point
	for _, s := range m.Solids {
		units = append(units, s.Polygon...)
		if c := s.Circle; c != nil {
			units = append(units, around(c.Center, c.Radius)...)
		}
		if c := s.Capsule; c != nil {
			units = append(units, around(c.Center1, c.Radius)...)
			units = append(units, around(c.Center2, c.Radius)...)
		}
	}
	for _, section := range m.sections() {
		for _, l := range section.loops {
			units = append(units, l.Points...)
		}
	}
	for _, o := range m.Objects {
		units = append(units, o.At)
	}
	if len(units) == 0 {
		return box2d.Vec2{}, box2d.Vec2{}, false
	}
	low, high := units[0], units[0]
	for _, p := range units[1:] {
		low = Point{min(low[0], p[0]), min(low[1], p[1])}
		high = Point{max(high[0], p[0]), max(high[1], p[1])}
	}
	return low.Vec2(), high.Vec2(), true
}

// around is the corners of the box around a circle at center of radius, in units.
func around(center Point, radius int32) []Point {
	return []Point{{center[0] - radius, center[1] - radius}, {center[0] + radius, center[1] + radius}}
}
