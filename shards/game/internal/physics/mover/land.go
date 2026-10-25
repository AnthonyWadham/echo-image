package mover

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// LandReach is the farthest Land moves a circle, in meters. A map wall must be thinner than twice this,
// or a circle in its middle can't land.
const LandReach = 6.0

// landStep is the spacing of the grid Land searches: a power of two, so every grid offset is exact.
const landStep = 0.25

// landGrid is every grid offset within LandReach, nearest first, ties in a fixed order. Built once.
var landGrid = func() []box2d.Vec2 {
	const n = int(LandReach / landStep)
	type cell struct{ i, j int }
	var cells []cell
	for i := -n; i <= n; i++ {
		for j := -n; j <= n; j++ {
			if i*i+j*j <= n*n {
				cells = append(cells, cell{i, j})
			}
		}
	}
	slices.SortFunc(cells, func(a, b cell) int {
		return cmp.Or(cmp.Compare(a.i*a.i+a.j*a.j, b.i*b.i+b.j*b.j), cmp.Compare(a.i, b.i), cmp.Compare(a.j, b.j))
	})
	grid := make([]box2d.Vec2, len(cells))
	for k, c := range cells {
		grid[k] = box2d.Vec2{X: float64(c.i) * landStep, Y: float64(c.j) * landStep}
	}
	return grid
}()

// Inside reports whether point lies in a shape filter accepts. Move can't push a circle out of a shape
// its center is in, so a caller lands it first.
func Inside(world *box2d.World, point box2d.Vec2, filter box2d.QueryFilter) bool {
	return overlaps(world, point, 0, filter)
}

// Land returns the nearest point to circle's center, on a grid within LandReach, where the whole circle
// clears the shapes filter accepts and the straight way there crosses no shape edge accepts (the map
// edge), or ok=false when there is none. Ties go in a fixed order, so every platform lands in the same
// spot. Call it outside a Box2D step, like Move.
func Land(world *box2d.World, circle box2d.Circle, filter, edge box2d.QueryFilter) (box2d.Vec2, bool) {
	for _, step := range landGrid {
		p := box2d.Vec2{X: circle.Center.X + step.X, Y: circle.Center.Y + step.Y}
		if !overlaps(world, p, circle.Radius, filter) && !crosses(world, circle.Center, step, edge) {
			return p, true
		}
	}
	return circle.Center, false
}

// overlaps reports whether a circle at center touches a shape filter accepts.
func overlaps(world *box2d.World, center box2d.Vec2, radius float64, filter box2d.QueryFilter) bool {
	proxy := box2d.MakeProxy([]box2d.Vec2{center}, 1, radius)
	hit := false
	world.OverlapShape(&proxy, filter, func(box2d.ShapeID, any) bool {
		hit = true
		return false // one is enough
	}, nil)
	return hit
}

// crosses reports whether the segment from origin along translation touches a shape filter accepts.
func crosses(world *box2d.World, origin, translation box2d.Vec2, filter box2d.QueryFilter) bool {
	hit := false
	world.CastRay(origin, translation, filter, func(box2d.ShapeID, box2d.Vec2, box2d.Vec2, float64, any) float64 {
		hit = true
		return 0 // one is enough
	}, nil)
	return hit
}
