package physics

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/interact"
)

// A shape's Box2D category bits are its kind (interact.Kind), never its team. A shape's CategoryBits
// says "what I am", MaskBits says "what I collide with"; two shapes interact only if each one's
// category is in the other's mask (the cheapest place to stop pairs you'd otherwise classify and
// discard). Friend/foe is decided from Owner and Team components in gameplay code, NOT baked into
// these bits (see CLAUDE.md → Collision filtering & teams).
//
// A map shape's category is its map kind (wall, bounds) and its mask is all bits, so each query's mask
// decides what stops it.

// MoveBlockers is the map kinds that stop a Body. A pass effect drops kinds from its holder's.
const MoveBlockers = interact.Wall | interact.Bounds

// Match reports whether a contact is between kinds x and y in either fixture order, testing category
// MEMBERSHIP with a bitwise-AND (not ==) so a shape carrying more than one category bit still
// classifies correctly. On a hit it returns the two entities remapped to (x, y) order. A shape in
// multiple categories can satisfy more than one Match call, so callers order their checks and return
// early to make that a priority rather than a double-handle.
func Match(p *physics2d.ContactEventPayload, x, y interact.Kind) (xe, ye cardinal.EntityID, ok bool) {
	switch {
	case p.FilterA.CategoryBits&uint64(x) != 0 && p.FilterB.CategoryBits&uint64(y) != 0:
		return p.EntityA, p.EntityB, true
	case p.FilterB.CategoryBits&uint64(x) != 0 && p.FilterA.CategoryBits&uint64(y) != 0:
		return p.EntityB, p.EntityA, true
	default:
		return 0, 0, false
	}
}
