package effect

import (
	"fmt"

	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
)

// EffectTuning is data-driven effect config, loaded once at boot from content/effect/tuning.json. It
// says what each effect kind does, whoever applies it; the applier sends the numbers in EffectIntent.
// Read it via data.Get[EffectTuning]().
type EffectTuning struct {
	Kinds map[string]KindDef `json:"kinds"` // by effect kind
}

// KindDef is what an effect kind does. The load fails on a name it doesn't know.
type KindDef struct {
	Tags     interact.Tags `json:"tags"`     // states it grants the holder: "stunned", "cc", "ends_on_cc", "unstoppable"
	Passes   interact.Kind `json:"passes"`   // map kinds the holder moves through: only "wall"
	Stat     string        `json:"stat"`     // the stat Value changes: "move_speed", or "" for none
	Stacking string        `json:"stacking"` // "merge", "per_source" or "instanced"
}

// Name is the data-catalog key used by data.Get[EffectTuning]().
func (EffectTuning) Name() string { return "effect_tuning" }

// JSONFile is the path within the content embed.FS.
func (EffectTuning) JSONFile() string { return "effect/tuning.json" }

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t EffectTuning) Validate() error {
	for kind, def := range t.Kinds {
		if _, _, err := def.resolve(kind); err != nil {
			return err
		}
	}
	return nil
}

// entry returns kind's entry, without numbers, and its stacking rule, or ok=false for an unknown kind.
func (t EffectTuning) entry(kind string) (status.Entry, status.Stacking, bool) {
	def, ok := t.Kinds[kind]
	if !ok {
		return status.Entry{}, 0, false
	}
	e, rule, err := def.resolve(kind)
	return e, rule, err == nil
}

// resolve turns def into an entry for kind and its stacking rule.
func (def KindDef) resolve(kind string) (status.Entry, status.Stacking, error) {
	unknown := func(what, name string) error {
		return fmt.Errorf("effect_tuning: kind %q: unknown %s %q", kind, what, name)
	}
	e := status.Entry{Kind: kind, Tags: def.Tags, Pass: def.Passes}
	if def.Passes&^interact.Wall != 0 {
		return e, 0, fmt.Errorf("effect_tuning: kind %q: a Body can move through walls only", kind)
	}
	var ok bool
	if e.Stat, ok = statNamed(def.Stat); !ok {
		return e, 0, unknown("stat", def.Stat)
	}
	rule, ok := stackingNamed(def.Stacking)
	if !ok {
		return e, 0, unknown("stacking", def.Stacking)
	}
	return e, rule, nil
}

func statNamed(name string) (status.Stat, bool) {
	switch name {
	case "":
		return status.NoStat, true
	case "move_speed":
		return status.MoveSpeed, true
	}
	return 0, false
}

func stackingNamed(name string) (status.Stacking, bool) {
	switch name {
	case "merge":
		return status.Merge, true
	case "per_source":
		return status.PerSource, true
	case "instanced":
		return status.Instanced, true
	}
	return 0, false
}
