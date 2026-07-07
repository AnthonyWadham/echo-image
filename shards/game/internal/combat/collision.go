package combat

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/physics"
)

// DamageIntent is a deferred request to apply damage to a target entity. CollisionSystem emits
// these while classifying contacts; DamageResolveSystem drains them and applies the result. The
// point of the indirection is that nothing here mutates game state — see routeContact.
type DamageIntent struct {
	Target cardinal.EntityID
	Source cardinal.EntityID
	Amount int
}

// Name is the system-event name.
func (DamageIntent) Name() string { return "damage_intent" }

// CollisionSystemState receives the physics2d begin streams and emits gameplay intents. It reads
// ContactBegin (solid touches) and TriggerBegin (sensor overlaps) because either can carry a
// gameplay collision depending on whether a shape is a sensor.
type CollisionSystemState struct {
	cardinal.BaseSystemState

	ContactBegin cardinal.WithSystemEventReceiver[physics2d.ContactBeginEvent]
	TriggerBegin cardinal.WithSystemEventReceiver[physics2d.TriggerBeginEvent]
	Damage       cardinal.WithSystemEventEmitter[DamageIntent]
}

// CollisionSystem is the single classification pass over this tick's contacts. It runs on Update,
// after the physics plugin's PreUpdate step has produced the events. Contacts and triggers are
// handled in one interleaved pass with no per-type batching — see routeContact for why that's safe.
func CollisionSystem(state *CollisionSystemState) {
	// Read config once at system entry — a single catalog lookup per tick, not one per contact.
	tuning := data.Get[CombatTuning]()
	for evt := range state.ContactBegin.Iter() {
		routeContact(state, tuning, evt.ContactEventPayload)
	}
	for evt := range state.TriggerBegin.Iter() {
		routeContact(state, tuning, evt.ContactEventPayload)
	}
}

// routeContact classifies one contact/overlap by collider category and emits the matching intent.
// It deliberately never touches game state: every effect is a deferred DamageIntent. Because the
// only mutation happens later in DamageResolveSystem — and damage sums commutatively there — the
// order these events arrive in (and whether solid contacts and sensor triggers interleave) cannot
// change the outcome. That property is exactly what lets us skip the per-type event fan-out that
// rampage used; we don't need "all projectiles, then all enemies" when order is irrelevant.
//
// Classification is on Box2D category bits (integer compares), never a reverse ECS type lookup,
// so it's effectively free next to the physics solve that produced the contact.
func routeContact(state *CollisionSystemState, tuning CombatTuning, p physics2d.ContactEventPayload) {
	if proj, enemy, ok := match(p, physics.CategoryProjectile, physics.CategoryEnemy); ok {
		state.Damage.Emit(DamageIntent{Target: enemy, Source: proj, Amount: tuning.ProjectileDamage})
		return
	}
	// Enemy touches player. NOTE: this fires on contact *begin* only, so a player held in sustained
	// overlap takes one hit per touch, not continuous damage. When the player feature lands, choose a
	// model — a per-enemy contact-damage cooldown, or a per-tick "still overlapping" check via the
	// physics ActiveContacts / OverlapAABB query — and emit DamageIntent on that cadence instead.
	if enemy, player, ok := match(p, physics.CategoryEnemy, physics.CategoryPlayer); ok {
		state.Damage.Emit(DamageIntent{Target: player, Source: enemy, Amount: tuning.EnemyContactDamage})
		return
	}

	// Unclassified pair: log during bring-up so a category/mask you haven't wired yet is visible
	// instead of silently ignored. Drop this once the switch above covers everything real.
	state.Logger().Debug().
		Uint64("cat_a", p.FilterA.CategoryBits).
		Uint64("cat_b", p.FilterB.CategoryBits).
		Msg("collision: unclassified contact")
}

// match reports whether a contact is between categories x and y in either fixture order. It tests
// category *membership* with a bitwise-AND (not ==), so a shape carrying more than one category
// bit (e.g. CategoryEnemy|CategoryElite) still classifies correctly. On a hit it returns the
// entities remapped to (x, y) order, so the caller gets a stable (kind-x, kind-y) pair regardless
// of which side Box2D put where. A shape in multiple categories can satisfy more than one match()
// call; routeContact's ordered checks plus early return make that a priority, not a double-emit.
func match(p physics2d.ContactEventPayload, x, y uint64) (xe, ye cardinal.EntityID, ok bool) {
	switch {
	case p.FilterA.CategoryBits&x != 0 && p.FilterB.CategoryBits&y != 0:
		return p.EntityA, p.EntityB, true
	case p.FilterB.CategoryBits&x != 0 && p.FilterA.CategoryBits&y != 0:
		return p.EntityB, p.EntityA, true
	default:
		return 0, 0, false
	}
}
