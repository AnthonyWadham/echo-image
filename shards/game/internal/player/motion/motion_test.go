package motion_test

import (
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics/mover"
	"echo_image/shards/game/internal/player/motion"
)

// A Body in a wall deeper than a landing reaches is stuck, and Step says so. One nearer its face lands.
func TestStepSaysWhenABodyFindsNowhereToLand(t *testing.T) {
	def := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&def)
	bodyDef := box2d.DefaultBodyDef()
	half := mover.LandReach + 1
	polygon := box2d.MakeBox(half, half)
	shape := box2d.DefaultShapeDef()
	shape.Filter.CategoryBits = uint64(interact.Wall)
	world.CreatePolygonShape(world.CreateBody(&bodyDef), &shape, &polygon)
	for _, c := range []struct {
		x     float64
		stuck bool
	}{{0, true}, {half - 1, false}} {
		body := motion.Body{Center: box2d.Vec2{X: c.x}, Radius: 0.5}
		if _, _, stuck := motion.Step(world, body, status.Status{}, 0.05); stuck != c.stuck {
			t.Errorf("a Body %v m into a %v m wall: stuck %v, want %v", half-c.x, 2*half, stuck, c.stuck)
		}
	}
}

// ell is a world with an L-shaped wall, as a map builds one: a chain outline that stops a walk, and two
// fill boxes that meet on a seam at x = 2, under the bottom face.
func ell() *box2d.World {
	def := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&def)
	bodyDef := box2d.DefaultBodyDef()
	body := world.CreateBody(&bodyDef)
	chain := box2d.DefaultChainDef()
	chain.Points = []box2d.Vec2{{X: 0, Y: 0}, {X: 6, Y: 0}, {X: 6, Y: 2}, {X: 2, Y: 2}, {X: 2, Y: 6}, {X: 0, Y: 6}}
	chain.IsLoop = true
	chain.Filter.CategoryBits = uint64(interact.Wall)
	world.CreateChain(body, &chain)
	shape := box2d.DefaultShapeDef()
	shape.Filter.CategoryBits = uint64(interact.Fill)
	for _, box := range []box2d.Polygon{
		box2d.MakeOffsetBox(1, 3, box2d.Vec2{X: 1, Y: 3}, box2d.RotIdentity),
		box2d.MakeOffsetBox(2, 1, box2d.Vec2{X: 4, Y: 1}, box2d.RotIdentity),
	} {
		world.CreatePolygonShape(body, &shape, &box)
	}
	return world
}

// A Body pressed into an outlined wall slides along it over the seam between its fill pieces, either
// way and however hard it presses, as far each tick as on a wall with no seam: only the outline stops a
// walk. Two solid pieces under one face hook it there, one way, when it presses hard.
func TestStepSlidesOverTheSeamOfAWallsFill(t *testing.T) {
	const dt, speed = 0.05, 5.0
	world := ell()
	for _, along := range []float64{1, -1} {
		for _, press := range []float64{45, 75, 85} { // degrees from the wall
			sin, cos := math.Sincos(press * math.Pi / 180)
			b := motion.Body{Center: box2d.Vec2{X: 2 - along*0.3, Y: -0.5}, Radius: 0.5, Speed: speed,
				Move: box2d.Vec2{X: along * cos, Y: sin}}
			want := speed * dt * cos
			for tick := range int(0.6/want) + 1 { // from 0.3 m before the seam at x = 2 to 0.3 m past it
				at, _, _ := motion.Step(world, b, status.Status{}, dt)
				if moved := (at.X - b.Center.X) * along; math.Abs(moved-want) > 1e-9 {
					t.Fatalf("sliding %+.0f, pressed at %v°, tick %d at %v: it moved %v along the wall, want %v",
						along, press, tick, b.Center, moved, want)
				}
				b.Center = at
			}
		}
	}
}

// A Body whose center is in an outlined wall's fill, too deep to touch the outline, lands outside.
func TestStepLandsABodyOutOfAWallsFill(t *testing.T) {
	b := motion.Body{Center: box2d.Vec2{X: 1, Y: 4}, Radius: 0.5}
	at, _, stuck := motion.Step(ell(), b, status.Status{}, 0.05)
	// Its edge would touch the left face 1.5 m back, so 1.75 m back is the nearest clear cell.
	if want := (box2d.Vec2{X: -0.75, Y: 4}); stuck || at != want {
		t.Errorf("it is at %v (stuck %v), want it landed at %v", at, stuck, want)
	}
	// With a pass through walls it stays: their fill is theirs to pass too.
	if at, _, _ := motion.Step(ell(), b, status.Status{Pass: interact.Wall}, 0.05); at != b.Center {
		t.Errorf("passing walls, it moved to %v", at)
	}
}
