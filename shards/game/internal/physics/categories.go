package physics

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Collision category bits describe what a shape IS (its kind) — never its team. A shape's
// CategoryBits says "what I am", MaskBits says "what I collide with"; two shapes interact only if
// each one's category is in the other's mask (the cheapest place to stop pairs you'd otherwise
// classify and discard). Friend/foe is decided from Team components in gameplay code, NOT baked into
// these bits (see CLAUDE.md → Collision filtering & teams). Add a bit here per new kind.
//
// This vocabulary is the game's cross-cutting collision spine — every feature that spawns a body
// imports it — so it lives in its own file, kept out of the plugin wiring in physics.go
// (see CLAUDE.md → File layout within a package).
const (
	CategoryEnvironment uint64 = 1 << iota // static walls / obstacles; mask CategoryProjectile to stop shots
	CategoryBody                           // player Body: invincible, blocks shots, never a damage target
	CategoryEcho                           // player Echo: carries Health; the damageable target
	CategoryProjectile                     // shots: not Box2D bodies; projectile sweeps them (query filter only)
)

// Match reports whether a contact is between category kinds x and y in either fixture order, testing
// category MEMBERSHIP with a bitwise-AND (not ==) so a shape carrying more than one category bit still
// classifies correctly. On a hit it returns the two entities remapped to (x, y) order. A shape in
// multiple categories can satisfy more than one Match call, so callers order their checks and return
// early to make that a priority rather than a double-handle.
func Match(p *physics2d.ContactEventPayload, x, y uint64) (xe, ye cardinal.EntityID, ok bool) {
	switch {
	case p.FilterA.CategoryBits&x != 0 && p.FilterB.CategoryBits&y != 0:
		return p.EntityA, p.EntityB, true
	case p.FilterB.CategoryBits&x != 0 && p.FilterA.CategoryBits&y != 0:
		return p.EntityB, p.EntityA, true
	default:
		return 0, 0, false
	}
}
