package area

import (
	"encoding/json"
	"fmt"

	"echo_image/shards/game/internal/interact"
)

// AreaTuning is data-driven area config, loaded once at boot from content/area/tuning.json.
type AreaTuning struct {
	Kinds map[string]KindDef `json:"kinds"` // by area kind ID
}

// KindDef is what an area kind does: for each thing that overlaps it, the first rule that matches.
type KindDef struct {
	Rules []Rule `json:"rules"`
}

// Rule is what an area does each tick a thing its filter matches overlaps it.
type Rule struct {
	interact.Filter
	To              To      `json:"to"`
	DamagePerSecond float64 `json:"damage_per_second"` // continuous, so scaled by the tick
}

// To is who a rule acts on.
type To uint8

const (
	_    To = iota // content must name one
	Self           // the area's holder
)

// meetable is the kinds an area can meet: the things AreaSystem tests for overlap.
const meetable = interact.Body | interact.Echo

// JSONFile is the path within the content embed.FS.
func (AreaTuning) JSONFile() string { return "area/tuning.json" }

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t AreaTuning) Validate() error {
	for id, k := range t.Kinds {
		for i, r := range k.Rules {
			switch {
			case r.To == 0:
				return fmt.Errorf("area_tuning: %q rule %d needs a to", id, i)
			case r.DamagePerSecond < 0:
				return fmt.Errorf("area_tuning: %q rule %d: damage_per_second must be >= 0", id, i)
			case r.Kinds&^meetable != 0:
				return fmt.Errorf("area_tuning: %q rule %d names a kind an area never meets", id, i)
			}
		}
	}
	return nil
}

// meets returns the kinds k's rules can match, so its query skips the rest.
func (k KindDef) meets() interact.Kind {
	var kinds interact.Kind
	for _, r := range k.Rules {
		if r.Kinds == 0 {
			return meetable
		}
		kinds |= r.Kinds
	}
	return kinds
}

// UnmarshalJSON reads who a rule acts on. An unknown name fails, so bad content fails the boot.
func (t *To) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err != nil {
		return err
	}
	if name != "self" {
		return fmt.Errorf("unknown to %q", name)
	}
	*t = Self
	return nil
}
