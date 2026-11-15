package mover

import (
	"fmt"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// wall is every shape's category here; blocked is the filter the circle moves with.
const wall = 1

var blocked = box2d.QueryFilter{CategoryBits: 2, MaskBits: wall}

func v(x, y float64) box2d.Vec2 { return box2d.Vec2{X: x, Y: y} }

func at(p box2d.Vec2) func(float64) box2d.Vec2 { return func(float64) box2d.Vec2 { return p } }

func box(x0, y0, x1, y1 float64) []box2d.Vec2 {
	return []box2d.Vec2{v(x0, y0), v(x1, y0), v(x1, y1), v(x0, y1)}
}

// newWorld returns a world that is never stepped, and a static body at position to put walls on.
func newWorld(position box2d.Vec2) (*box2d.World, box2d.BodyID) {
	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Position = position
	return world, world.CreateBody(&bodyDef)
}

// addLoop adds a chain loop through points. Counter-clockwise, it blocks from outside.
func addLoop(world *box2d.World, body box2d.BodyID, points ...box2d.Vec2) {
	def := box2d.DefaultChainDef()
	def.Points = points
	def.IsLoop = true
	def.Filter.CategoryBits = wall
	world.CreateChain(body, &def)
}

// addPolygon adds the convex polygon around points.
func addPolygon(world *box2d.World, body box2d.BodyID, points ...box2d.Vec2) {
	hull := box2d.ComputeHull(points)
	polygon := box2d.MakePolygon(&hull, 0)
	def := box2d.DefaultShapeDef()
	def.Filter.CategoryBits = wall
	world.CreatePolygonShape(body, &def, &polygon)
}

// A slide presses a circle into a wall at an angle while it moves along it. The circle starts touching
// the wall at from, t is along the wall and n out of it. A slide out of a corner first walks into it.
type slide struct {
	name   string
	from   func(radius float64) box2d.Vec2
	t, n   box2d.Vec2
	length float64
	solid  [][]box2d.Vec2 // convex pieces with the outline's shape here, and no seam on the way
	corner bool
	wedged float64 // from this press angle on, a valley holds the circle, so there's no slide
}

type outline struct {
	name   string
	points []box2d.Vec2 // counter-clockwise
	slides []slide
}

func outlines() []outline {
	x, y, nx, ny := v(1, 0), v(0, 1), v(-1, 0), v(0, -1)
	square := [][]box2d.Vec2{box(0, 0, 6, 6)}
	arms := [][]box2d.Vec2{box(0, 0, 6, 2), box(0, 0, 2, 6)}
	tee := [][]box2d.Vec2{box(-6, 4, 6, 6), box(-1, 0, 1, 5)}
	all := []outline{
		{"L", []box2d.Vec2{v(0, 0), v(6, 0), v(6, 2), v(2, 2), v(2, 6), v(0, 6)}, []slide{
			{"bottom, round a corner", at(v(0.2, 0)), nx, ny, 0.4, square, false, 0},
			{"left, round a corner", at(v(0, 0.2)), ny, nx, 0.4, square, false, 0},
			{"arm, into the inner corner", func(r float64) box2d.Vec2 { return v(2+r+0.3, 2) }, nx, y, 0.3, arms, false, 0},
			{"stem, into the inner corner", func(r float64) box2d.Vec2 { return v(2, 2+r+0.3) }, ny, x, 0.3, arms, false, 0},
			{"arm, out of the inner corner", func(r float64) box2d.Vec2 { return v(2+r, 2) }, x, y, 0.4, arms, true, 0},
			{"top, round a corner", at(v(1.8, 6)), x, y, 0.4, [][]box2d.Vec2{box(0, 0, 2, 6)}, false, 0},
		}},
		{"T", []box2d.Vec2{v(-1, 0), v(1, 0), v(1, 4), v(6, 4), v(6, 6), v(-6, 6), v(-6, 4), v(-1, 4)}, []slide{
			{"top, over the stem", at(v(-1.2, 6)), x, y, 0.4, tee, false, 0},
			{"under the bar, into a corner", func(r float64) box2d.Vec2 { return v(-1-r-0.3, 4) }, x, ny, 0.3, tee, false, 0},
			{"stem, into a corner", func(r float64) box2d.Vec2 { return v(-1, 4-r-0.3) }, y, nx, 0.3, tee, false, 0},
			{"under the bar, out of a corner", func(r float64) box2d.Vec2 { return v(-1-r, 4) }, nx, ny, 0.4, tee, true, 0},
			{"stem, out of a corner", func(r float64) box2d.Vec2 { return v(1, 4-r) }, ny, x, 0.4, tee, true, 0},
			{"top, round the bar's end", at(v(-5.8, 6)), nx, y, 0.4, tee, false, 0},
		}},
		{"straight wall in three segments", []box2d.Vec2{v(-6, -2), v(6, -2), v(6, 0), v(0.5, 0), v(-0.5, 0), v(-6, 0)}, []slide{
			{"+x over both joints", at(v(-0.7, 0)), x, y, 1.4, [][]box2d.Vec2{box(-6, -2, 6, 0)}, false, 0},
			{"-x over both joints", at(v(0.7, 0)), nx, y, 1.4, [][]box2d.Vec2{box(-6, -2, 6, 0)}, false, 0},
		}},
	}
	for _, deg := range []float64{2, 5, 10, 20, 45, 70} {
		c, s := math.Cos(deg*math.Pi/180), math.Sin(deg*math.Pi/180)
		e := v(-3*c, -3*s) // a slope down and left from a ridge at the origin
		ridge := []box2d.Vec2{v(e.X, -6), v(6, -6), v(6, 0), v(0, 0), e}
		all = append(all, outline{fmt.Sprintf("%g° ridge", deg), ridge, []slide{
			{"flat, over the ridge", at(v(1, 0)), nx, y, 2, [][]box2d.Vec2{ridge}, false, 0},
			{"slope, over the ridge", at(box2d.MulSV(0.5, e)), v(c, s), v(-s, c), 2.5, [][]box2d.Vec2{ridge}, false, 0},
		}})
		u := v(3*c, 3*s) // a slope up and right from a valley at the origin
		valley := [][]box2d.Vec2{box(-3, -6, 0, 0), {v(0, -6), v(u.X, -6), u, v(0, 0)}}
		all = append(all, outline{fmt.Sprintf("%g° valley", deg), []box2d.Vec2{v(-3, -6), v(u.X, -6), u, v(0, 0), v(-3, 0)}, []slide{
			{"flat, into the valley", at(v(-1, 0)), x, y, 2, valley, false, 90 - deg},
			{"slope, into the valley", at(box2d.MulSV(0.5, u)), v(-c, -s), v(-s, c), 2.5, valley, false, 90 - deg},
		}})
	}
	return all
}

// A circle slides along a chain loop as it does along the solid the loop outlines: through corners and
// over joints, at every press angle, radius, speed and tick rate, a tick that moves it along the solid
// moves it along the chain too. Not where a valley wedges it: Box2D's plane solver jitters there, on both.
func TestChainSlidesLikeTheSolidItOutlines(t *testing.T) {
	for _, o := range outlines() {
		chain, body := newWorld(box2d.Vec2{})
		addLoop(chain, body, o.points...)
		for _, s := range o.slides {
			solid, body := newWorld(box2d.Vec2{})
			for _, piece := range s.solid {
				addPolygon(solid, body, piece...)
			}
			for _, angle := range []float64{0, 15, 30, 45, 60, 75, 80, 84, 86, 88, 89, 89.9} {
				if s.wedged > 0 && angle >= s.wedged {
					continue
				}
				for _, radius := range []float64{0.3, 0.5, 1} {
					for _, speed := range []float64{5, 12.5} {
						for _, hz := range []float64{20, 60} {
							if at, ok := slideBoth(chain, solid, s, angle, radius, speed, 1/hz); !ok {
								t.Errorf("%s, %s: pressed at %g°, radius %g, %g m/s, %g Hz: the chain stopped it at %v",
									o.name, s.name, angle, radius, speed, hz, at)
							}
						}
					}
				}
			}
		}
	}
}

// slideBoth runs s in both worlds. At the first tick that moves the circle along the solid but not along
// the chain, it returns where the chain's circle was and false.
func slideBoth(chain, solid *box2d.World, s slide, angle, radius, speed, dt float64) (box2d.Vec2, bool) {
	move := func(world *box2d.World, center, velocity box2d.Vec2) box2d.Vec2 {
		center, _ = Move(world, box2d.Circle{Center: center, Radius: radius}, velocity, dt, blocked)
		return center
	}
	c := box2d.MulAdd(s.from(radius), radius, s.n)
	b := c
	if s.corner {
		// From half a meter out, as play would come in.
		c = box2d.Add(c, box2d.MulSV(0.5, box2d.Add(s.t, s.n)))
		b = c
		in := box2d.MulSV(-5, box2d.Normalize(box2d.Add(s.t, s.n)))
		for range 30 {
			c, b = move(chain, c, in), move(solid, b, in)
		}
	}
	start := b
	cos, sin := math.Cos(angle*math.Pi/180), math.Sin(angle*math.Pi/180)
	velocity := box2d.MulSV(speed, box2d.Sub(box2d.MulSV(cos, s.t), box2d.MulSV(sin, s.n)))
	for range int(math.Ceil(s.length/(speed*cos*dt))) + 40 {
		nc, nb := move(chain, c, velocity), move(solid, b, velocity)
		if box2d.Dot(box2d.Sub(nb, b), s.t) > 1e-9 && box2d.Dot(box2d.Sub(nc, c), s.t) <= 1e-12 {
			return c, false
		}
		c, b = nc, nb
		if box2d.Dot(box2d.Sub(b, start), s.t) >= s.length {
			break
		}
	}
	return c, true
}

// A chain loop blocks only from its front, as Box2D's chains do: counter-clockwise, a circle outside
// stops at it, and one inside walks out. The loop's body is off the origin, so the rule must read the
// circle in the body's frame.
func TestChainBlocksFromTheFrontOnly(t *testing.T) {
	world, body := newWorld(v(10, 0))
	addLoop(world, body, v(0, 0), v(4, 0), v(4, 4), v(0, 4)) // x from 10 to 14
	const radius = 0.5
	walk := func(world *box2d.World, center, velocity box2d.Vec2) box2d.Vec2 {
		for range 20 {
			center, _ = Move(world, box2d.Circle{Center: center, Radius: radius}, velocity, 1.0/20, blocked)
		}
		return center
	}
	if c := walk(world, v(8, 2), v(5, 0)); math.Abs(c.X+radius-10) > 2*box2d.LinearSlop {
		t.Errorf("from outside, it stopped at %v, want its edge at the face, x 10", c)
	}
	if c := walk(world, v(12, 2), v(5, 0)); c.X-radius <= 14 {
		t.Errorf("from inside, it stopped at %v, want it past the face at x 14", c)
	}

	// Inside an L, beside its inner corner, the center is in front of the arm's top face and past its end.
	// Box2D gives no plane there and the cast lets it creep, so it gets out late, not never.
	ell, body := newWorld(v(0, 0))
	addLoop(ell, body, v(0, 0), v(6, 0), v(6, 2), v(2, 2), v(2, 6), v(0, 6))
	if c := walk(ell, v(1.9, 2.3), v(0, -5)); c.Y+radius >= 0 {
		t.Errorf("from inside the L, it stopped at %v, want it out below the bottom at y 0", c)
	}
}
