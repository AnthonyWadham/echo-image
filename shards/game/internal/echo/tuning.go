package echo

import (
	"fmt"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/physics/timestep"
)

// EchoTuning is data-driven Echo config, loaded once at boot from content/echo/tuning.json. Each
// champion's Echo has its own delay and area. Durations are in SECONDS and converted to ticks at
// runtime so the mechanic is tick-rate independent (see CLAUDE.md → Tick-rate independence).
type EchoTuning struct {
	MaxHP       float64            `json:"max_hp"`       // Echo starting / max health
	HitRadius   float64            `json:"hit_radius"`   // Echo collider radius — its hurtbox, in world units
	DeathEffect string             `json:"death_effect"` // the effect kind an Echo's death puts on its Body: out
	Champions   map[string]EchoDef `json:"champions"`    // by champion ID
}

// EchoDef is a champion's Echo.
type EchoDef struct {
	DelaySeconds float64 `json:"delay_seconds"` // how far the Echo trails the Body
	Area         string  `json:"area"`          // its area kind, a key of area.AreaTuning.Kinds, or "" for none
}

// JSONFile is the path within the content embed.FS.
func (EchoTuning) JSONFile() string { return "echo/tuning.json" }

// DelayTicks converts the champion's delay to whole ticks at the current server rate.
func (d EchoDef) DelayTicks() int {
	return timestep.Ticks(d.DelaySeconds)
}

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t EchoTuning) Validate() error {
	if t.MaxHP <= 0 || t.HitRadius <= 0 || t.DeathEffect == "" {
		return fmt.Errorf("echo_tuning: need max_hp > 0, hit_radius > 0 and a death_effect, got %+v", t)
	}
	for id, d := range t.Champions {
		if d.DelayTicks() < 1 {
			return fmt.Errorf("echo_tuning: %q's delay_seconds %.3f rounds to %d ticks at %d Hz; need >= 1 tick",
				id, d.DelaySeconds, d.DelayTicks(), physics.ServerTickRateHz)
		}
	}
	return nil
}
