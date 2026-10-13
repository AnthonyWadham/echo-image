package combat

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/team"
)

// CollisionSystem is combat's single pass over this tick's physics2d begin streams (Update, after the
// physics PreUpdate step). For each contact it recognizes as a combat kind it decides friend/foe and
// emits a DamageIntent — it never mutates game state (that's DamageResolveSystem's job). It is NOT the
// global contact router: it reacts only to damage-producing pairs and ignores everything else (walls,
// other features' triggers). A different feature that reacts to contacts reads the same broadcast
// stream in its own system.
type CollisionSystem struct {
	Data *data.Plugin
}

func (s *CollisionSystem) Run(w *cardinal.World) {
	tuning := data.Get[CombatTuning](s.Data) // once at entry, not per contact
	for evt := range w.SystemEvents[physics2d.ContactBeginEvent]() {
		routeContact(w, tuning, &evt.ContactEventPayload)
	}
	for evt := range w.SystemEvents[physics2d.TriggerBeginEvent]() {
		routeContact(w, tuning, &evt.ContactEventPayload)
	}
}

// routeContact emits the damage intent for one contact when it's a combat pair. Kind is matched on
// category bits (physics.Match); whether the contact is an ATTACK vs. friendly fire is decided from
// Team state (team.SameTeam). Contacts that aren't combat's kinds fall through untouched — this system
// reacts to damage, it does not classify the whole world.
func routeContact(w *cardinal.World, tuning CombatTuning, p *physics2d.ContactEventPayload) {
	// Projectile strikes an Echo → damage, unless they share a team (no friendly fire).
	if proj, echo, ok := physics.Match(p, physics.CategoryProjectile, physics.CategoryEcho); ok {
		if !team.SameTeam(w, proj, echo) {
			w.EmitSystemEvent(DamageIntent{Target: echo, Source: proj, Amount: tuning.ProjectileDamage})
		}
		return
	}

	// Anti-camp (GDD §3, an Echo overlapping its OWN Body self-damages) is deliberately NOT handled here.
	// The Echo (BodyTypeManual) and Body (kinematic) never form a solid contact — physics2d pairs only
	// dynamic-involved bodies — and the Echo masks only projectiles, so no ContactBegin fires for the
	// pair; it is also a sustained per-tick effect, not a one-shot begin event. M3 adds it as a per-tick
	// distance check (dist(Echo, own Body) < radius) that emits a DamageIntent. Do NOT add a
	// physics.Match(CategoryEcho, CategoryBody) branch here — it would never fire.
}
