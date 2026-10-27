package status

import (
	"cmp"
	"fmt"

	"echo_image/shards/game/internal/interact"
)

// Tuning is what each effect kind does, whoever applies it, loaded from content/effect/tuning.json.
// The applier sends the numbers in an Intent.
type Tuning struct {
	Kinds map[string]KindDef `json:"kinds"` // by effect kind
}

// KindDef is what an effect kind does. The load fails on a name it doesn't know.
type KindDef struct {
	Tags     interact.Tags `json:"tags"`     // states it grants the holder: "stunned", "cc", "ends_on_cc", "unstoppable"
	Passes   interact.Kind `json:"passes"`   // map kinds the holder moves through: only "wall"
	Stat     string        `json:"stat"`     // the stat Value changes: "move_speed", or "" for none
	Stacking string        `json:"stacking"` // "merge", "per_source" or "instanced"
}

// Intent asks for an effect of kind Kind on Target, from the tick it is applied at, for Ticks ticks.
// The kind's tags, passes, stat and stacking come from Tuning; the numbers come from here.
type Intent struct {
	Target     uint32
	Kind       string // a key of Tuning.Kinds
	Ticks      uint64
	Value      int32   // the kind's stat, in basis points
	VelX, VelY float64 // forced velocity, units per second
	Source     uint32
}

// Name is the data-catalog key used by data.Get[effect.EffectTuning]().
func (Tuning) Name() string { return "effect_tuning" }

// JSONFile is the path within the content embed.FS.
func (Tuning) JSONFile() string { return "effect/tuning.json" }

// Validate runs at load time; a returned error panics the boot — fail loud on bad content.
func (t Tuning) Validate() error {
	for kind, def := range t.Kinds {
		if _, _, err := def.resolve(kind); err != nil {
			return err
		}
	}
	return nil
}

// Compare orders intents by target, then CC last, so an Unstoppable added the same tick blocks it.
// The rest of the key keeps emit order out of it, since that can change after a restore.
func (t Tuning) Compare(a, b Intent) int {
	return cmp.Or(
		cmp.Compare(a.Target, b.Target),
		cmp.Compare(t.ccRank(a.Kind), t.ccRank(b.Kind)),
		cmp.Compare(a.Source, b.Source),
		cmp.Compare(a.Kind, b.Kind),
		cmp.Compare(a.Ticks, b.Ticks),
		cmp.Compare(a.Value, b.Value),
		cmp.Compare(a.VelX, b.VelX),
		cmp.Compare(a.VelY, b.VelY),
	)
}

// Apply adds in to list from tick on, and returns the new list, or ok=false for a kind t doesn't know.
func (t Tuning) Apply(list []Entry, next *uint32, in Intent, tick uint64) ([]Entry, bool) {
	e, rule, ok := t.entry(in.Kind)
	if !ok {
		return list, false
	}
	e.Value, e.VelX, e.VelY, e.Source = in.Value, in.VelX, in.VelY, in.Source
	list, _ = Add(list, next, e, rule, tick, in.Ticks)
	return list, true
}

// ccRank is 1 for a CC kind and 0 for any other.
func (t Tuning) ccRank(kind string) int {
	if e, _, _ := t.entry(kind); e.Tags&interact.CC != 0 {
		return 1
	}
	return 0
}

// entry returns kind's entry, without numbers, and its stacking rule, or ok=false for an unknown kind.
func (t Tuning) entry(kind string) (Entry, Stacking, bool) {
	def, ok := t.Kinds[kind]
	if !ok {
		return Entry{}, 0, false
	}
	e, rule, err := def.resolve(kind)
	return e, rule, err == nil
}

// resolve turns def into an entry for kind and its stacking rule.
func (def KindDef) resolve(kind string) (Entry, Stacking, error) {
	unknown := func(what, name string) error {
		return fmt.Errorf("effect_tuning: kind %q: unknown %s %q", kind, what, name)
	}
	e := Entry{Kind: kind, Tags: def.Tags, Pass: def.Passes}
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

func statNamed(name string) (Stat, bool) {
	switch name {
	case "":
		return NoStat, true
	case "move_speed":
		return MoveSpeed, true
	}
	return 0, false
}

func stackingNamed(name string) (Stacking, bool) {
	switch name {
	case "merge":
		return Merge, true
	case "per_source":
		return PerSource, true
	case "instanced":
		return Instanced, true
	}
	return 0, false
}
