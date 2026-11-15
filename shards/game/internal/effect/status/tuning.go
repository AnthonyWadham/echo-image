package status

import (
	"cmp"
	"fmt"
	"slices"

	"echo_image/shards/game/internal/interact"
)

// Tuning is what each effect kind does, whoever applies it, loaded from content/effect/tuning.json.
// The applier sends the numbers in an Intent.
type Tuning struct {
	Kinds map[string]KindDef `json:"kinds"` // by effect kind
}

// KindDef is what an effect kind does. The load fails on a name it doesn't know.
type KindDef struct {
	Tags      interact.Tags `json:"tags"`       // states it grants the holder: "stunned", "cc", "ends_on_cc", "unstoppable"
	Passes    interact.Kind `json:"passes"`     // map kinds the holder moves through: only "wall"
	Stat      Stat          `json:"stat"`       // the stat Value changes: "move_speed", or "" for none
	Stacking  *Stacking     `json:"stacking"`   // "merge", "per_source" or "instanced"
	Ends      interact.Tags `json:"ends"`       // it removes the holder's entries with any of these tags as it lands
	BlockedBy interact.Tags `json:"blocked_by"` // it doesn't land on a holder with any of these tags
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

// Compare orders intents by target, then the kinds that read the holder's tags last, so an unstoppable
// added the same tick blocks a stun and a root ends a dash, whatever their sources. The rest of the key
// keeps emit order out of it, since that can change after a restore.
func (t Tuning) Compare(a, b Intent) int {
	return cmp.Or(
		cmp.Compare(a.Target, b.Target),
		cmp.Compare(t.readsTags(a.Kind), t.readsTags(b.Kind)),
		cmp.Compare(a.Source, b.Source),
		cmp.Compare(a.Kind, b.Kind),
		cmp.Compare(a.Ticks, b.Ticks),
		cmp.Compare(a.Value, b.Value),
		cmp.Compare(a.VelX, b.VelX),
		cmp.Compare(a.VelY, b.VelY),
	)
}

// Apply adds in to list from tick on, by its kind's rules, and returns the new list, or ok=false for a
// kind t doesn't know. It doesn't land on a holder with a tag its kind is blocked by, and it ends the
// entries with a tag its kind ends.
func (t Tuning) Apply(list []Entry, next *uint32, in Intent, tick uint64) ([]Entry, bool) {
	e, rule, ok := t.entry(in.Kind)
	if !ok {
		return list, false
	}
	def := t.Kinds[in.Kind]
	if Fold(slices.Values(list), tick).Tags&def.BlockedBy != 0 {
		return list, true
	}
	if def.Ends != 0 {
		list = Remove(list, func(o Entry) bool { return o.Tags&def.Ends != 0 })
	}
	e.Value, e.VelX, e.VelY, e.Source = in.Value, in.VelX, in.VelY, in.Source
	return Add(list, next, e, rule, tick, in.Ticks), true
}

// readsTags is 1 for a kind a tag can block or that ends tagged entries, and 0 for any other.
func (t Tuning) readsTags(kind string) int {
	if k := t.Kinds[kind]; k.BlockedBy|k.Ends != 0 {
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
	e := Entry{Kind: kind, Tags: def.Tags, Pass: def.Passes, Stat: def.Stat}
	if def.Passes&^interact.Wall != 0 {
		return e, 0, fmt.Errorf("effect_tuning: kind %q: a Body can move through walls only", kind)
	}
	if def.Stacking == nil {
		return e, 0, fmt.Errorf("effect_tuning: kind %q: no stacking", kind)
	}
	return e, *def.Stacking, nil
}

var (
	statNames     = map[string]Stat{"": NoStat, "move_speed": MoveSpeed}
	stackingNames = map[string]Stacking{"merge": Merge, "per_source": PerSource, "instanced": Instanced}
)

// UnmarshalJSON reads a stat name. An unknown one fails, so bad content fails the boot.
func (s *Stat) UnmarshalJSON(b []byte) error { return interact.Name(b, "stat", statNames, s) }

// UnmarshalJSON reads a stacking name. An unknown one fails, so bad content fails the boot.
func (s *Stacking) UnmarshalJSON(b []byte) error {
	return interact.Name(b, "stacking", stackingNames, s)
}
