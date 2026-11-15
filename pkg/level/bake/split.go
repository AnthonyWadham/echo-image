package bake

import (
	"errors"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
)

// split cuts a shape's outline, counter-clockwise, into triangles between its corners, each a polygon a
// map file takes: the first such cut in a fixed order, so every machine cuts alike. A shape has none when
// a part of it is so thin that every cut leaves a sliver there.
//
// It tries every triangulation (O'Rourke, Computational Geometry in C, 1.6 and 1.4): cut[i][j] is the
// corner that makes a triangle with i and j and leaves both sides of it cut too.
func split(p []level.Point) ([][]level.Point, error) {
	n := len(p)
	side := func(i, j int) bool { return j == i+1 || i == 0 && j == n-1 || diagonal(p, i, j) }
	piece := func(i, k, j int) bool {
		_, err := level.Hull([]level.Point{p[i], p[k], p[j]})
		return err == nil
	}
	cut := make([][]int, n)
	for i := range cut {
		cut[i] = make([]int, n)
	}
	for span := 2; span < n; span++ {
		for i := 0; i+span < n; i++ {
			j := i + span
			cut[i][j] = -1
			for k := i + 1; k < j; k++ {
				if (k == i+1 || cut[i][k] >= 0) && (j == k+1 || cut[k][j] >= 0) && side(i, k) && side(k, j) &&
					piece(i, k, j) {
					cut[i][j] = k
					break
				}
			}
		}
	}
	if cut[0][n-1] < 0 {
		return nil, errors.New("can't be cut into pieces Box2D takes: part of it is so thin that every cut " +
			"leaves a piece under about 1 cm wide. Add a corner across from the thin part, or draw two shapes")
	}
	var pieces [][]level.Point
	var take func(i, j int)
	take = func(i, j int) {
		if j-i < 2 {
			return
		}
		k := cut[i][j]
		triangle, _ := level.Hull([]level.Point{p[i], p[k], p[j]})
		pieces = append(pieces, triangle)
		take(i, k)
		take(k, j)
	}
	take(0, n-1)
	return pieces, nil
}

// diagonal reports whether the segment between corners a and b runs inside the outline and clear of its
// sides: it leaves both corners inward, and stays more than LinearSlop from each side it doesn't end on.
func diagonal(p []level.Point, a, b int) bool {
	if !inward(p, a, b) || !inward(p, b, a) {
		return false
	}
	n := len(p)
	for c := range n {
		d := (c + 1) % n
		if c == a || c == b || d == a || d == b {
			continue
		}
		gap := box2d.SegmentDistance(p[a].Vec2(), p[b].Vec2(), p[c].Vec2(), p[d].Vec2())
		if gap.DistanceSquared <= box2d.LinearSlop*box2d.LinearSlop {
			return false
		}
	}
	return true
}

// inward reports whether the way from corner a to b starts inside the outline.
func inward(p []level.Point, a, b int) bool {
	n := len(p)
	before, after := p[(a+n-1)%n], p[(a+1)%n]
	if left(p[a], after, before) >= 0 { // a convex corner
		return left(p[a], p[b], before) > 0 && left(p[b], p[a], after) > 0
	}
	return !(left(p[a], p[b], after) >= 0 && left(p[b], p[a], before) >= 0)
}

// left is positive when c is left of the way from a to b, negative when right, 0 on its line. Points
// are in Box2D's world, so the products fit an int64.
func left(a, b, c level.Point) int64 {
	return (int64(b[0])-int64(a[0]))*(int64(c[1])-int64(a[1])) - (int64(b[1])-int64(a[1]))*(int64(c[0])-int64(a[0]))
}
