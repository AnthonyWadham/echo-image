package cast

import (
	"encoding/json"
	"fmt"

	"echo_image/shards/game/internal/ability/hooks"
	"echo_image/shards/game/internal/physics/timestep"
)

// Tuning is the ability rows, loaded from content/ability/tuning.json: the kits, how each ability is
// cast, and the hook that does it with its params. Bind binds the hooks after loading.
type Tuning struct {
	Kits      map[string][]string `json:"kits"`      // by champion ID: its ability IDs, slot 0 first
	Abilities map[string]Def      `json:"abilities"` // by ability ID
	bound     map[string]hooks.Hook
}

// Def is how one ability is cast, and what it does.
type Def struct {
	CooldownSeconds float64         `json:"cooldown_seconds"` // from a cast until the next
	Aimed           bool            `json:"aimed"`            // needs an aim direction to cast
	Hook            string          `json:"hook"`             // what it does: a hook name in ability/hooks
	Params          json.RawMessage `json:"params"`           // the hook's numbers and IDs
}

// Name is the data-catalog key used by data.Get[ability.AbilityTuning]().
func (Tuning) Name() string { return "ability_tuning" }

// JSONFile is the path within the content embed.FS.
func (Tuning) JSONFile() string { return "ability/tuning.json" }

// CooldownTicks converts CooldownSeconds to ticks.
func (d Def) CooldownTicks() uint64 {
	return uint64(timestep.Ticks(d.CooldownSeconds))
}

// Bind binds each ability's hook to its params. A missing or bad hook name or a bad param is an
// error, and so is a hook that needs aim on an ability that isn't aimed.
func (t *Tuning) Bind() error {
	t.bound = map[string]hooks.Hook{}
	for id, d := range t.Abilities {
		h, err := hooks.Bind(d.Hook, d.Params, timestep.Rate)
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

// Validate checks every cooldown, and that every kit names known abilities.
func (t Tuning) Validate() error {
	for id, d := range t.Abilities {
		if d.CooldownSeconds < 0 {
			return fmt.Errorf("ability_tuning: %q cooldown_seconds must be >= 0, got %v", id, d.CooldownSeconds)
		}
	}
	for champion, kit := range t.Kits {
		for _, id := range kit {
			if _, ok := t.Abilities[id]; !ok {
				return fmt.Errorf("ability_tuning: %q's kit names unknown ability %q", champion, id)
			}
		}
	}
	return nil
}
