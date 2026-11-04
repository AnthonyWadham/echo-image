// Package component is track's leaf: the record a constructor adds and any feature reads, without
// importing track's systems.
package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/track/past"
)

// Track is a thing's rows from its last ticks, as each tick's target sync showed it. Its constructor
// adds it; then every screen draws the thing and every judge finds it. Only track.RecordSystem writes it.
type Track struct {
	Rows      cardinal.Slice[past.Row] // a ring: tick t's row at t % len, if kept
	Predicted bool                     // its owner's client predicts it (a Body, an Echo)
}

// Name is the ECS component name.
func (Track) Name() string { return "track" }

// At is the row of tick, if it is kept.
func (t Track) At(tick uint64) (past.Row, bool) {
	n := t.Rows.Len()
	if n == 0 {
		return past.Row{}, false
	}
	if r := t.Rows.At(int(tick % uint64(n))); r.Tick == tick {
		return r, true
	}
	return past.Row{}, false
}

// Record is t with r in its tick's slot of keep, in place: Set it after. A new keep (its first row, or a
// window change) sizes the ring once and keeps the rows it still spans. An empty slot holds r, whose
// tick isn't its own, so At never takes it for a row.
func (t Track) Record(r past.Row, keep int) Track {
	if t.Rows.Len() != keep {
		rows := cardinal.SliceOf(r).Repeat(keep)
		for tick := r.Tick - min(r.Tick, uint64(keep-1)); tick < r.Tick; tick++ {
			if old, ok := t.At(tick); ok {
				rows = rows.With(int(tick%uint64(keep)), old)
			}
		}
		t.Rows = rows
	}
	t.Rows = t.Rows.With(int(r.Tick%uint64(keep)), r)
	return t
}

// AppendRows appends t's kept rows of the ticks before before that the ring spans, oldest first.
func (t Track) AppendRows(dst []past.Row, before uint64) []past.Row {
	for tick := before - min(before, uint64(t.Rows.Len())); tick < before; tick++ {
		if r, ok := t.At(tick); ok {
			dst = append(dst, r)
		}
	}
	return dst
}
