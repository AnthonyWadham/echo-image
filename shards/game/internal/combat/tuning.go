package combat

import "fmt"

// CombatTuning is data-driven combat balance, loaded once at boot from content/combat/tuning.json
// by the data plugin. Read it via data.Get[CombatTuning]() — never hardcode these numbers in a
// system. It's a Singleton (one JSON object, not an array of records).
//
// This is the template for a feature's config: the schema lives in the feature package that uses
// it, the JSON lives under content/combat/, and cross-feature references (when they come) are by
// string ID resolved through the catalog — never a Go import into another feature's types.
type CombatTuning struct {
	ProjectileDamage    float64 `json:"projectile_damage"`      // damage per projectile hit (discrete, per-event)
	SelfDamagePerSecond float64 `json:"self_damage_per_second"` // anti-camp DoT while an Echo overlaps its own Body
	OverlapRadius       float64 `json:"overlap_radius"`         // Echo↔own-Body distance under which anti-camp bites
}

// Name is the data-catalog key used by data.Get[CombatTuning]().
func (CombatTuning) Name() string { return "combat_tuning" }

// JSONFile is the path within the content embed.FS.
func (CombatTuning) JSONFile() string { return "combat/tuning.json" }

// Validate runs at load time; returning an error panics the boot — fail loud on bad content
// rather than shipping a quietly-broken balance number into the tick loop.
func (t CombatTuning) Validate() error {
	if t.ProjectileDamage < 0 || t.SelfDamagePerSecond < 0 || t.OverlapRadius < 0 {
		return fmt.Errorf("combat_tuning: projectile_damage/self_damage_per_second/overlap_radius must be >= 0, got %+v", t)
	}
	return nil
}
