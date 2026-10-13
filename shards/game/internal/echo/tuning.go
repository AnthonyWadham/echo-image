package echo

import (
	"fmt"
	"math"

	"echo_image/shards/game/internal/physics"
)

// EchoTuning is data-driven Echo config, loaded once at boot from content/echo/tuning.json. Read it via
// data.Get[EchoTuning](). Durations are in SECONDS and converted to ticks at runtime so the mechanic is
// tick-rate independent (see CLAUDE.md → Tick-rate independence). Singleton.
type EchoTuning struct {
	DelaySeconds float64 `json:"delay_seconds"` // how far the Echo trails the Body
	MaxHP        float64 `json:"max_hp"`        // Echo starting / max health
	HitRadius    float64 `json:"hit_radius"`    // Echo collider radius — its hurtbox, in world units
}

// Name is the data-catalog key used by data.Get[EchoTuning]().
func (EchoTuning) Name() string { return "echo_tuning" }

// JSONFile is the path within the content embed.FS.
func (EchoTuning) JSONFile() string { return "echo/tuning.json" }

// DelayTicks converts the configured delay to whole ticks at the current server rate.
func (t EchoTuning) DelayTicks() int {
	return int(math.Round(t.DelaySeconds * float64(physics.ServerTickRateHz)))
}

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t EchoTuning) Validate() error {
	if t.DelaySeconds < 0 || t.MaxHP <= 0 || t.HitRadius <= 0 {
		return fmt.Errorf("echo_tuning: need delay_seconds >= 0, max_hp > 0, hit_radius > 0, got %+v", t)
	}
	if d := t.DelayTicks(); d < 1 {
		return fmt.Errorf("echo_tuning: delay_seconds %.3f rounds to %d ticks at %d Hz; need >= 1 tick",
			t.DelaySeconds, d, physics.ServerTickRateHz)
	}
	return nil
}
