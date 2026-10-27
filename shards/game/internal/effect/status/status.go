// Package status is the Cardinal-free core of effects: what each kind does (Tuning), the entry an
// effect list holds, the rules that add one, and the fold that turns the active entries into what
// other rules read. It imports only the standard library and interact, so a client predictor built
// for wasm can run it.
package status

import (
	"iter"
	"slices"

	"echo_image/shards/game/internal/interact"
)

// Stat is a number the fold combines, with one fixed rule per stat.
type Stat uint8

const (
	NoStat    Stat = iota
	MoveSpeed      // Value in basis points: hastes add, only the strongest slow counts
)

// Stacking is what adding a kind does when an active entry of it is already there. A refresh keeps
// whichever ends later, so a short effect never cuts a long one short, in any order.
type Stacking uint8

const (
	Merge     Stacking = iota // one per kind: refresh it
	PerSource                 // one per kind and source: refresh that one
	Instanced                 // always a new entry
)

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
}

// Active reports whether e applies at tick.
func (e Entry) Active(tick uint64) bool { return e.Start <= tick && tick < e.End }

// Over reports whether e applies at no tick from tick on.
func (e Entry) Over(tick uint64) bool { return e.End <= tick }

// Status is the fold of the entries active at one tick.
type Status struct {
	Tags       interact.Tags
	Pass       interact.Kind
	Haste      int32   // summed, basis points
	Slow       int32   // strongest, basis points
	VelX, VelY float64 // summed forced velocity
	Forced     bool
}

// Fold combines the entries active at tick, in list order.
func Fold(entries iter.Seq[Entry], tick uint64) Status {
	var s Status
	for e := range entries {
		if !e.Active(tick) {
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

// Speed is base after hastes and the strongest slow, exactly base with neither. Stunned is zero.
func (s Status) Speed(base float64) float64 {
	if s.Tags&interact.Stunned != 0 {
		return 0
	}
	bps := (10000 + int64(s.Haste)) * (10000 - int64(min(s.Slow, 10000))) / 10000
	return base * (float64(bps) / 10000)
}

// Add applies e to list from tick for ticks ticks, and returns the new list, or ok=false when an
// active Unstoppable blocks it. The caller sets every field of e but Start, End and Serial.
func Add(list []Entry, next *uint32, e Entry, rule Stacking, tick, ticks uint64) ([]Entry, bool) {
	if e.Tags&interact.CC != 0 {
		if Fold(slices.Values(list), tick).Tags&interact.Unstoppable != 0 {
			return list, false
		}
		list = Remove(list, func(o Entry) bool { return o.Tags&interact.EndsOnCC != 0 })
	}
	e.Start, e.End = tick, tick+ticks
	if rule != Instanced {
		for i, o := range list {
			if o.Kind == e.Kind && (rule == Merge || o.Source == e.Source) && o.Active(tick) {
				if e.End > o.End {
					e.Serial = o.Serial
					list[i] = e
				}
				return list, true
			}
		}
	}
	*next++
	e.Serial = *next
	return append(list, e), true
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
