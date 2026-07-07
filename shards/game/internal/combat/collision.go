package combat

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/team"
)

// DamageIntent is a deferred request to apply damage to a target entity. CollisionSystem emits these
// while classifying contacts; DamageResolveSystem drains them and applies the result. Nothing here
// mutates game state — see routeContact.
type DamageIntent struct {
	Target cardinal.EntityID
	Source cardinal.EntityID
	Amount int
}

// Name is the system-event name.
func (DamageIntent) Name() string { return "damage_intent" }

// CollisionSystemState receives the physics2d begin streams, reads allegiance for friend/foe, and
// emits gameplay intents.
type CollisionSystemState struct {
	cardinal.BaseSystemState

	ContactBegin cardinal.WithSystemEventReceiver[physics2d.ContactBeginEvent]
	TriggerBegin cardinal.WithSystemEventReceiver[physics2d.TriggerBeginEvent]
	Damage       cardinal.WithSystemEventEmitter[DamageIntent]
	Teams        cardinal.Contains[team.Row] // allegiance lookup for friend/foe (read, never write)
}

// CollisionSystem is the single classification pass over this tick's contacts (Update, after the
// physics PreUpdate step). It classifies each contact by KIND (category bits) and decides friend/foe
// from Team components — kind lives in the bits, allegiance lives in the components (see CLAUDE.md).
func CollisionSystem(state *CollisionSystemState) {
	tuning := data.Get[CombatTuning]() // once at entry, not per contact
	for evt := range state.ContactBegin.Iter() {
		routeContact(state, tuning, evt.ContactEventPayload)
	}
	for evt := range state.TriggerBegin.Iter() {
		routeContact(state, tuning, evt.ContactEventPayload)
	}
}

// routeContact classifies one contact and emits the matching intent. Kind is matched on category
// bits; whether the contact is an ATTACK (vs. friendly fire or self-contact) is decided here from
// Team/owner state. It never mutates game state — all effects are deferred DamageIntents drained by
// DamageResolveSystem, so contact arrival order is irrelevant.
func routeContact(state *CollisionSystemState, tuning CombatTuning, p physics2d.ContactEventPayload) {
	// Projectile strikes an Echo → damage, unless they share a team (no friendly fire).
	if proj, echo, ok := match(p, physics.CategoryProjectile, physics.CategoryEcho); ok {
		if !state.sameTeam(proj, echo) {
			state.Damage.Emit(DamageIntent{Target: echo, Source: proj, Amount: tuning.ProjectileDamage})
		}
		return
	}

	// Anti-camp (GDD §3): an Echo overlapping its OWN Body self-damages. This is an OWNER match, not
	// a team match — a teammate's Body touching your Echo is NOT self-damage — so it needs the Echo's
	// owner ref, added when the Echo entity lands. The kind branch it will use:
	//   if echo, body, ok := match(p, physics.CategoryEcho, physics.CategoryBody); ok { ...owner check... }

	// Unclassified pair — logged during bring-up so an unwired category/mask is visible, not silent.
	state.Logger().Debug().
		Uint64("cat_a", p.FilterA.CategoryBits).
		Uint64("cat_b", p.FilterB.CategoryBits).
		Msg("collision: unclassified contact")
}

// sameTeam reports whether two entities share a Team. Entities without a Team (environment, etc.) are
// treated as teamless — never "same" — so neutral geometry can't accidentally suppress damage.
func (state *CollisionSystemState) sameTeam(a, b cardinal.EntityID) bool {
	ta, oka := state.teamOf(a)
	tb, okb := state.teamOf(b)
	return oka && okb && ta == tb
}

func (state *CollisionSystemState) teamOf(e cardinal.EntityID) (uint8, bool) {
	row, err := state.Teams.GetByID(e)
	if err != nil {
		return 0, false
	}
	return row.Team.Get().ID, true
}

// match reports whether a contact is between categories x and y in either fixture order, testing
// category MEMBERSHIP with a bitwise-AND (not ==) so a shape carrying more than one category bit
// still classifies correctly. On a hit it returns the entities remapped to (x, y) order. A shape in
// multiple categories can satisfy more than one call; routeContact's ordered checks plus early return
// make that a priority, not a double-emit.
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
