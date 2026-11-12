package component

import (
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/track/past"
)

// A track keeps each tick's row in its own slot, for as many ticks as it keeps. A window change keeps
// the rows the new ring spans, a ring is never read as a row it doesn't hold, and the rows before a tick
// stop short of it.
func TestTrackKeepsRowsByTick(t *testing.T) {
	row := func(tick uint64) past.Row {
		return past.Row{Tick: tick, Center: box2d.Vec2{X: float64(tick)}, Radius: 0.5, Until: 1}
	}
	ticks := func(tr Track, before uint64) []uint64 {
		var got []uint64
		for _, r := range tr.AppendRows(nil, before) {
			if r != row(r.Tick) {
				t.Fatalf("row of tick %d is %+v, want %+v", r.Tick, r, row(r.Tick))
			}
			got = append(got, r.Tick)
		}
		return got
	}
	check := func(what string, tr Track, keep int, before uint64, want ...uint64) {
		t.Helper()
		if got := ticks(tr, before); tr.Rows.Len() != keep || !slices.Equal(got, want) {
			t.Errorf("%s: %d slots holding ticks %v, want %d holding %v", what, tr.Rows.Len(), got, keep, want)
		}
		for tick := range before + 2 {
			if _, ok := tr.At(tick); ok != slices.Contains(want, tick) {
				t.Errorf("%s: At(%d) = %v, want %v", what, tick, ok, !ok)
			}
		}
	}
	first := Track{}.Record(row(6), 5)
	check("sized at its first row", first, 5, 7, 6)
	var tr Track
	for tick := range uint64(8) {
		tr = tr.Record(row(tick), 5)
	}
	check("kept", tr, 5, 8, 3, 4, 5, 6, 7)
	if got := ticks(tr, 6); !slices.Equal(got, []uint64{3, 4, 5}) {
		t.Errorf("rows before 6: ticks %v, want 3 4 5", got)
	}
	tr = tr.Record(row(8), 3)
	check("shrunk", tr, 3, 9, 6, 7, 8)
	tr = tr.Record(row(9), 6)
	check("grown", tr, 6, 10, 6, 7, 8, 9)
	tr = tr.Record(row(11), 6) // no row of tick 10
	check("a tick missed", tr, 6, 12, 6, 7, 8, 9, 11)
	tr = tr.Record(row(12), 6)
	check("its oldest dropped", tr, 6, 13, 7, 8, 9, 11, 12)
}
