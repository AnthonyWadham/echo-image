// Package motion is how a player Body moves each tick: the direction its input asks for, and the step
// through the map. player.MovementSystem runs it on the server, and the client predictor
// (internal/predict) runs the same code for its own Body. It imports no Cardinal, so it builds for wasm
// and gives the same bits there.
package motion

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics/mover"
)

// Blockers is the map kinds that stop a Body. A pass effect drops kinds from its holder's.
const Blockers = interact.Wall | interact.Bounds

// edge is what a landing never crosses: the map edge.
var edge = box2d.QueryFilter{CategoryBits: uint64(interact.Body), MaskBits: uint64(interact.Bounds)}

// Land is where a Body's circle, inside a blocker filter accepts, lands: the nearest clear spot whose way
// there doesn't cross the map edge, or ok=false with none within mover.LandReach.
func Land(world *box2d.World, circle box2d.Circle, filter box2d.QueryFilter) (box2d.Vec2, bool) {
	return mover.Land(world, circle, filter, edge)
}

// LandWhere is Land with clear saying where the Body fits, for a caller that already has those answers.
func LandWhere(world *box2d.World, center box2d.Vec2, clear func(box2d.Vec2) bool) (box2d.Vec2, bool) {
	return mover.LandWhere(world, center, edge, clear)
}

// Unit returns (x, y) scaled to length 1, or (0, 0) for a ~zero, huge or non-finite input, so a bad
// client frame never reaches physics or gameplay as NaN. Its length is box2d's, never math.Hypot,
// which fuses a multiply-add on arm64 and so gives other bits there than in the browser.
func Unit(x, y float64) (float64, float64) {
	mag := box2d.Length(box2d.Vec2{X: x, Y: y})
	if mag < 1e-9 || math.IsInf(mag, 0) || math.IsNaN(mag) {
		return 0, 0
	}
	return x / mag, y / mag
}

// Body is what one tick's move reads of a Body.
type Body struct {
	Center box2d.Vec2
	Radius float64
	Move   box2d.Vec2 // its input direction this tick: a unit vector, or zero
	Speed  float64    // its base move speed, units per second
}

// Step moves b through world for dt seconds under st, and returns its new center and its velocity
// clipped against what it touched. A forced velocity replaces input, the folded speed scales input (a
// stun is zero), and a pass drops map kinds from what stops it. A Body whose center is inside a
// blocker first lands on the nearest clear spot inside the map, since the mover would walk it out the
// far side. stuck says it found none: a map the boot's rules should have refused.
func Step(world *box2d.World, b Body, st status.Status, dt float64) (center, velocity box2d.Vec2, stuck bool) {
	filter := box2d.QueryFilter{CategoryBits: uint64(interact.Body), MaskBits: uint64(Blockers &^ st.Pass)}
	circle := box2d.Circle{Center: b.Center, Radius: b.Radius}
	if mover.Inside(world, circle.Center, filter) {
		var landed bool
		circle.Center, landed = Land(world, circle, filter)
		stuck = !landed
	}
	v := box2d.Vec2{X: st.VelX, Y: st.VelY}
	if !st.Forced {
		speed := st.Speed(b.Speed)
		v = box2d.Vec2{X: b.Move.X * speed, Y: b.Move.Y * speed}
	}
	center, velocity = mover.Move(world, circle, v, dt, filter)
	return center, velocity, stuck
}
