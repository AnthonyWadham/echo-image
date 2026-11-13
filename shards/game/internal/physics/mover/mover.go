// Package mover moves a circle through a Box2D world the way Box2D's geometric character mover does:
// it stops at the shapes a query filter accepts and slides along them. It imports box2d, not Cardinal,
// so a client predictor built for wasm can run the same code and get the same bits.
package mover

import (
	"cmp"
	"math"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const (
	passes  = 5    // collide, solve and cast at most this many times per move
	minStep = 0.01 // a pass that moves less than this many meters ends the move, unless it met a new wall
)

// Move moves circle by velocity for dt seconds through world, stopping at the shapes filter accepts
// and sliding along them. It returns the circle's new center, and velocity without the part that
// pushes into those shapes. Call it outside a Box2D step: a locked world reports no hits. A circle
// whose center starts inside a shape isn't pushed out, and can pass through it. A chain meets it as
// Box2D's chains collide: from the front only, and smoothly where two segments join.
//
// Same inputs give the same bits: the box2d helpers round every product (no FMA), and planes are
// solved in geometric order, since CollideMover reports them in the order the world built its tree.
func Move(world *box2d.World, circle box2d.Circle, velocity box2d.Vec2, dt float64,
	filter box2d.QueryFilter,
) (center, clipped box2d.Vec2) {
	center = circle.Center
	target := box2d.MulAdd(center, dt, velocity)
	var planes []box2d.CollisionPlane
	collect := func(shape box2d.ShapeID, hit *box2d.PlaneResult, _ any) bool {
		if world.ShapeType(shape) == box2d.ChainSegmentShape && !chainPlane(world, shape, center) {
			return true
		}
		// Rigid, like upstream's FLT_MAX push limit.
		planes = append(planes, box2d.CollisionPlane{Plane: hit.Plane, PushLimit: math.MaxFloat64, ClipVelocity: true})
		return true
	}
	for range passes {
		capsule := box2d.Capsule{Center1: center, Center2: center, Radius: circle.Radius}
		planes = planes[:0]
		world.CollideMover(&capsule, filter, collect, nil)
		slices.SortFunc(planes, byGeometry)
		solved := box2d.SolvePlanes(box2d.Sub(target, center), planes)
		fraction := world.CastMover(&capsule, solved.Translation, filter)
		step := box2d.MulSV(fraction, solved.Translation)
		center = box2d.Add(center, step)
		// A short pass that met a wall it has no plane for yet goes on, to collect it and slide.
		if box2d.LengthSquared(step) < minStep*minStep && (len(planes) > 0 || fraction >= 1) {
			break
		}
	}
	return center, box2d.ClipVector(velocity, planes)
}

// chainPlane reports whether a chain segment's plane counts: from its front only, and not at an end while
// center is over the next segment's face (Box2D's ghost vertices). The pinned CollideMover does neither.
func chainPlane(world *box2d.World, shape box2d.ShapeID, center box2d.Vec2) bool {
	seg := world.ShapeChainSegment(shape)
	c := box2d.InvTransformPoint(world.BodyTransform(world.ShapeBody(shape)), center)
	p1, p2 := seg.Segment.Point1, seg.Segment.Point2
	if !front(p1, p2, c) {
		return false
	}
	e := box2d.Sub(p2, p1)
	switch t := box2d.Dot(box2d.Sub(c, p1), e); {
	case t <= 0:
		return !overFace(seg.Ghost1, p1, c)
	case t >= box2d.Dot(e, e):
		return !overFace(p2, seg.Ghost2, c)
	}
	return true
}

// front reports whether c is on the side of the segment from a to b that collides.
func front(a, b, c box2d.Vec2) bool { return box2d.Cross(box2d.Sub(c, a), box2d.Sub(b, a)) >= 0 }

// overFace reports whether c is in front of the segment from a to b and projects strictly inside it.
func overFace(a, b, c box2d.Vec2) bool {
	e := box2d.Sub(b, a)
	t := box2d.Dot(box2d.Sub(c, a), e)
	return front(a, b, c) && t > 0 && t < box2d.Dot(e, e)
}

// byGeometry orders planes by normal, then offset.
func byGeometry(a, b box2d.CollisionPlane) int {
	return cmp.Or(
		cmp.Compare(a.Plane.Normal.X, b.Plane.Normal.X),
		cmp.Compare(a.Plane.Normal.Y, b.Plane.Normal.Y),
		cmp.Compare(a.Plane.Offset, b.Plane.Offset),
	)
}
