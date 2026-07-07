package combat

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/team"
)

// CollisionSystemState receives the physics2d begin streams and, for the contact kinds combat cares
// about, emits DamageIntents. It is NOT the global contact router: it reacts only to damage-producing
// pairs and ignores everything else (walls, other features' triggers). A different feature that reacts
// to contacts declares its own receiver over the same broadcast stream.
type CollisionSystemState struct {
	cardinal.BaseSystemState

	ContactBegin cardinal.WithSystemEventReceiver[physics2d.ContactBeginEvent]
	TriggerBegin cardinal.WithSystemEventReceiver[physics2d.TriggerBeginEvent]
	Damage       cardinal.WithSystemEventEmitter[DamageIntent]
	Teams        cardinal.Contains[team.Row] // allegiance lookup for friend/foe (read, never write)
}

// CollisionSystem is combat's single pass over this tick's contacts (Update, after the physics
// PreUpdate step). For each contact it recognizes as a combat kind it decides friend/foe and emits a
// DamageIntent — it never mutates game state (that's DamageResolveSystem's job).
func CollisionSystem(state *CollisionSystemState) {
	tuning := data.Get[CombatTuning]() // once at entry, not per contact
	for evt := range state.ContactBegin.Iter() {
		routeContact(state, tuning, &evt.ContactEventPayload)
	}
	for evt := range state.TriggerBegin.Iter() {
		routeContact(state, tuning, &evt.ContactEventPayload)
	}
}

// routeContact emits the damage intent for one contact when it's a combat pair. Kind is matched on
// category bits (physics.Match); whether the contact is an ATTACK vs. friendly fire is decided from
// Team state (team.SameTeam). Contacts that aren't combat's kinds fall through untouched — this system
// reacts to damage, it does not classify the whole world.
func routeContact(state *CollisionSystemState, tuning CombatTuning, p *physics2d.ContactEventPayload) {
	// Projectile strikes an Echo → damage, unless they share a team (no friendly fire).
	if proj, echo, ok := physics.Match(p, physics.CategoryProjectile, physics.CategoryEcho); ok {
		if !team.SameTeam(&state.Teams, proj, echo) {
			state.Damage.Emit(DamageIntent{Target: echo, Source: proj, Amount: tuning.ProjectileDamage})
		}
		return
	}

	// Anti-camp (GDD §3): an Echo overlapping its OWN Body self-damages. This is an OWNER match, not a
	// team match — a teammate's Body touching your Echo is NOT self-damage — so it needs the Echo's
	// owner ref, added when the Echo entity lands. The kind branch it will use:
	//   if echo, body, ok := physics.Match(p, physics.CategoryEcho, physics.CategoryBody); ok { ...owner check... }
}
