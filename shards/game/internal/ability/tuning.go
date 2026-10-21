package ability

import (
	"fmt"
	"math"

	"echo_image/shards/game/internal/physics"
	playercomp "echo_image/shards/game/internal/player/component"
)

// AbilityTuning is data-driven ability config, loaded once at boot from content/ability/tuning.json.
// It holds only what casting needs; each feature an ability changes keeps its own numbers under the
// same ability ID (content/projectile/ for a shot). Read it via data.Get[AbilityTuning]().
type AbilityTuning struct {
	DefaultKit []string              `json:"default_kit"` // ability IDs a claimed Body gets, slot 0 first
	Abilities  map[string]AbilityDef `json:"abilities"`   // by ability ID
}

// AbilityDef is how one ability is cast.
type AbilityDef struct {
	CooldownSeconds float64 `json:"cooldown_seconds"` // from a cast until the next
	Aimed           bool    `json:"aimed"`            // needs an aim direction to cast
}

// Name is the data-catalog key used by data.Get[AbilityTuning]().
func (AbilityTuning) Name() string { return "ability_tuning" }

// JSONFile is the path within the content embed.FS.
func (AbilityTuning) JSONFile() string { return "ability/tuning.json" }

// CooldownTicks converts CooldownSeconds to ticks.
func (d AbilityDef) CooldownTicks() uint64 {
	return uint64(math.Round(d.CooldownSeconds * float64(physics.ServerTickRateHz)))
}

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t AbilityTuning) Validate() error {
	for id, d := range t.Abilities {
		if d.CooldownSeconds < 0 {
			return fmt.Errorf("ability_tuning: %q cooldown_seconds must be >= 0, got %v", id, d.CooldownSeconds)
		}
	}
	if len(t.DefaultKit) > playercomp.ButtonCount {
		return fmt.Errorf("ability_tuning: default_kit has %d abilities, more than the %d input buttons",
			len(t.DefaultKit), playercomp.ButtonCount)
	}
	for _, id := range t.DefaultKit {
		if _, ok := t.Abilities[id]; !ok {
			return fmt.Errorf("ability_tuning: default_kit names unknown ability %q", id)
		}
	}
	return nil
}
