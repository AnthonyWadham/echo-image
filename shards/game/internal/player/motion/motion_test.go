package motion_test

import (
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
