package ability

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/internal/ability/hooks"
	"echo_image/shards/game/internal/physics"
	playercomp "echo_image/shards/game/internal/player/component"
)

// AbilityTuning is data-driven ability config, loaded once at boot from content/ability/tuning.json:
// the kits, how each ability is cast, and the hook that does it with its params. Read it via
// data.Get[AbilityTuning]().
type AbilityTuning struct {
	Kits      map[string][]string   `json:"kits"`      // by champion ID: its ability IDs, slot 0 first
	Abilities map[string]AbilityDef `json:"abilities"` // by ability ID
	bound     map[string]hooks.Hook // by ability ID, bound by Resolve
}

// AbilityDef is how one ability is cast, and what it does.
type AbilityDef struct {
	CooldownSeconds float64         `json:"cooldown_seconds"` // from a cast until the next
	Aimed           bool            `json:"aimed"`            // needs an aim direction to cast
	Hook            string          `json:"hook"`             // what it does: a hook name in ability/hooks
	Params          json.RawMessage `json:"params"`           // the hook's numbers and IDs
}

// Name is the data-catalog key used by data.Get[AbilityTuning]().
func (AbilityTuning) Name() string { return "ability_tuning" }

// JSONFile is the path within the content embed.FS.
func (AbilityTuning) JSONFile() string { return "ability/tuning.json" }

// CooldownTicks converts CooldownSeconds to ticks.
func (d AbilityDef) CooldownTicks() uint64 {
	return uint64(math.Round(d.CooldownSeconds * float64(physics.ServerTickRateHz)))
}

// Resolve binds each ability's hook to its params once, at load. A missing or bad hook name or a bad
// param fails the boot, and so does a hook that needs aim on an ability that isn't aimed.
func (t *AbilityTuning) Resolve(context.Context, data.Source) error {
	t.bound = map[string]hooks.Hook{}
	for id, d := range t.Abilities {
		h, err := hooks.Bind(d.Hook, d.Params, physics.ServerTickRateHz)
		if err != nil {
			return fmt.Errorf("ability_tuning: %q: %w", id, err)
		}
		if h.Aimed && !d.Aimed {
			return fmt.Errorf("ability_tuning: %q: hook %q needs an aimed ability", id, d.Hook)
		}
		t.bound[id] = h
	}
	return nil
}

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t AbilityTuning) Validate() error {
	for id, d := range t.Abilities {
		if d.CooldownSeconds < 0 {
			return fmt.Errorf("ability_tuning: %q cooldown_seconds must be >= 0, got %v", id, d.CooldownSeconds)
		}
	}
	for champion, kit := range t.Kits {
		if len(kit) > playercomp.ButtonCount {
			return fmt.Errorf("ability_tuning: %q's kit has %d abilities, more than the %d input buttons",
				champion, len(kit), playercomp.ButtonCount)
		}
		for _, id := range kit {
			if _, ok := t.Abilities[id]; !ok {
				return fmt.Errorf("ability_tuning: %q's kit names unknown ability %q", champion, id)
			}
		}
	}
	return nil
}
