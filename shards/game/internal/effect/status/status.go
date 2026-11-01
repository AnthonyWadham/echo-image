// Package status is the Cardinal-free core of effects: what each kind does (Tuning), the entry an
// effect list holds, the rules that add one, and the fold that turns the active entries into what
// other rules read. It imports only the standard library and interact, so a client predictor built
// for wasm can run it.
package status

import (
	"iter"
	"slices"
	"strings"

	"echo_image/shards/game/internal/interact"
)

// Stat is a number the fold combines, with one fixed rule per stat.
type Stat uint8

const (
	NoStat    Stat = iota
	MoveSpeed      // Value in basis points: hastes add, only the strongest slow counts
)

// Stacking is what adding a kind does when an active entry with its refresh key is already there:
// its kind, and its source for PerSource. A refresh keeps what is strongest at every tick, so a short
// effect never cuts a long one short, and a weak one never erases a strong one, in any order.
type Stacking uint8

const (
	Merge     Stacking = iota // one per kind: refresh it
	PerSource                 // one per kind and source: refresh that one
	Instanced                 // always a new entry
)

// Forever is the Ticks of an effect that lasts until something removes it: out, until the round ends.
const Forever = ^uint64(0)

// Entry is one timed effect. It holds its resolved numbers, so the fold needs no catalog.
type Entry struct {
	Kind       string
	Tags       interact.Tags
	Pass       interact.Kind // map kinds the holder moves through
	Stat       Stat
	Value      int32   // basis points
	VelX, VelY float64 // forced velocity, units per second
	Source     uint32  // the entity that applied it
	Start, End uint64  // active for Start <= tick < End
	Serial     uint32  // unique in its list, since entity IDs are reused
	Stacking   Stacking
}

// Active reports whether e applies at tick.
func (e Entry) Active(tick uint64) bool { return e.Start <= tick && tick < e.End }

// Over reports whether e applies at no tick from tick on.
func (e Entry) Over(tick uint64) bool { return e.End <= tick }

// strength is how much e's number does: a stun or a pass has none, so only its end counts.
func (e Entry) strength() int64 { return max(int64(e.Value), -int64(e.Value)) }

// refreshes reports whether e and o share a refresh key.
func (e Entry) refreshes(o Entry) bool {
	return e.Stacking != Instanced && e.Kind == o.Kind && (e.Stacking == Merge || e.Source == o.Source)
}

// covers reports whether active entry e makes o add nothing: as strong, and lasting as long.
func (e Entry) covers(o Entry, tick uint64) bool {
	return e.refreshes(o) && e.Active(tick) && e.strength() >= o.strength() && e.End >= o.End
}

// Shown is an effect kind as other screens see it, active until End. Its numbers and source stay with
// its holder.
type Shown struct {
	Kind string
	End  uint64
}

// ShownAt is the kinds active at tick, in kind order, each until its last entry ends.
func ShownAt(entries iter.Seq[Entry], tick uint64) []Shown {
	var shown []Shown
	for e := range entries {
		if !e.Active(tick) {
			continue
		}
		if i := slices.IndexFunc(shown, func(s Shown) bool { return s.Kind == e.Kind }); i >= 0 {
			shown[i].End = max(shown[i].End, e.End)
		} else {
			shown = append(shown, Shown{Kind: e.Kind, End: e.End})
		}
	}
	slices.SortFunc(shown, func(a, b Shown) int { return strings.Compare(a.Kind, b.Kind) })
	return shown
}

// Status is the fold of the entries active at one tick.
type Status struct {
	Tags       interact.Tags
	Pass       interact.Kind
	Haste      int32   // summed, basis points
	Slow       int32   // strongest, basis points
	VelX, VelY float64 // summed forced velocity
	Forced     bool
}

// Fold combines the entries active at tick, in list order: per refresh key, only the strongest (the
// first of equals).
func Fold(entries iter.Seq[Entry], tick uint64) Status {
	var s Status
	i := 0
	for e := range entries {
		i++
		if !e.Active(tick) || outranked(entries, e, i, tick) {
			continue
		}
		s.Tags |= e.Tags
		s.Pass |= e.Pass
		if e.Stat == MoveSpeed {
			if e.Value >= 0 {
				s.Haste += e.Value
			} else {
				s.Slow = max(s.Slow, -e.Value)
			}
		}
		if e.VelX != 0 || e.VelY != 0 {
			s.VelX, s.VelY, s.Forced = s.VelX+e.VelX, s.VelY+e.VelY, true
		}
	}
	return s
}

// outranked reports whether an active entry with e's refresh key, other than the at-th, is stronger
// than e, or as strong and earlier in the list.
func outranked(entries iter.Seq[Entry], e Entry, at int, tick uint64) bool {
	if e.Stacking == Instanced {
		return false
	}
	j := 0
	for o := range entries {
		j++
		if j != at && o.Active(tick) && e.refreshes(o) &&
			(o.strength() > e.strength() || o.strength() == e.strength() && j < at) {
			return true
		}
	}
	return false
}

// Speed is base after hastes and the strongest slow, exactly base with neither. Stunned is zero.
func (s Status) Speed(base float64) float64 {
	if s.Tags&interact.Stunned != 0 {
		return 0
	}
	bps := (10000 + int64(s.Haste)) * (10000 - int64(min(s.Slow, 10000))) / 10000
	return base * (float64(bps) / 10000)
}

// Add applies e to list from tick for ticks ticks by rule, and returns the new list. The caller sets
// every field of e but Start, End, Serial and Stacking. What a kind ends or is blocked by is Apply's.
func Add(list []Entry, next *uint32, e Entry, rule Stacking, tick, ticks uint64) []Entry {
	e.Start, e.End, e.Stacking = tick, tick+min(ticks, Forever-tick), rule
	if slices.ContainsFunc(list, func(o Entry) bool { return o.covers(e, tick) }) {
		return list
	}
	// e takes the place, and the serial, of the first entry it covers, and the rest it covers go. An
	// entry stronger but shorter, or weaker but longer, stays beside it: Fold counts the strongest.
	kept, placed := list[:0], false
	for _, o := range list {
		switch {
		case !e.covers(o, tick):
			kept = append(kept, o)
		case !placed:
			e.Serial, placed = o.Serial, true
			kept = append(kept, e)
		}
	}
	if placed {
		return kept
	}
	*next++
	e.Serial = *next
	return append(list, e)
}

// Remove drops the entries match reports, keeping the order of the rest.
func Remove(list []Entry, match func(Entry) bool) []Entry {
	out := list[:0]
	for _, e := range list {
		if !match(e) {
			out = append(out, e)
		}
	}
	return out
}

// Sweep drops the entries that are over at tick.
func Sweep(list []Entry, tick uint64) []Entry {
	return Remove(list, func(e Entry) bool { return e.Over(tick) })
}
