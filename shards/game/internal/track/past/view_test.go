package past

import (
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics/timestep"
)

// A target at a view is drawn as the client draws it: its newer row, partway from its older one. One
// that died in the newer tick moves at its own pace to where it died, and is gone after. Every view in
// (10, 11] is drawn from rows 10 and 11: a whole tick is that tick's end.
func TestAtViewDrawsAsTheClientDoes(t *testing.T) {
	older := Row{Tick: 10, Center: box2d.Vec2{X: 1, Y: 2}, Radius: 0.5, Tags: interact.Stunned, Until: 1}
	at := func(x, y float64) *box2d.Vec2 { return &box2d.Vec2{X: x, Y: y} }
	for _, c := range []struct {
		name     string
		hasOlder bool
		until    float64 // the newer row's
		view     float64
		want     *box2d.Vec2 // nil if gone
	}{
		{"a quarter of the way", true, 1, 10.25, at(1.5, 1)},
		{"at the newer tick", true, 1, 11, at(3, -2)},
		{"new at the newer tick", false, 1, 10.25, at(3, -2)},
		{"a quarter of the way, dying halfway", true, 0.5, 10.25, at(2, 0)},
		{"where it died", true, 0.5, 10.5, at(3, -2)},
		{"after it died", true, 0.5, 10.75, nil},
		{"dead as the tick began", true, 0, math.Nextafter(10, 11), nil},
		{"new, after it died", false, 0.5, 10.75, nil},
	} {
		if a, b := RowTicks(c.view); a != 10 || b != 11 {
			t.Fatalf("%s: RowTicks(%v) = %d, %d, want 10, 11", c.name, c.view, a, b)
		}
		newer := Row{Tick: 11, Center: box2d.Vec2{X: 3, Y: -2}, Radius: 0.5, Tags: interact.CC, Until: c.until}
		got, ok := AtView(older, c.hasOlder, newer, c.view)
		want := newer
		if c.want != nil {
			want.Center = *c.want
		}
		if ok != (c.want != nil) || ok && got != want {
			t.Errorf("%s: %+v %v, want %+v %v", c.name, got, ok, want, c.want != nil)
		}
	}
}

// A shot's tick judged at view v is drawn across views v to v + 1: each span of it shows the target as
// AtView draws those views, from the pair of rows they are drawn from. One that died shows until it did,
// and one that is new from where it first is.
func TestAcrossShowsTheTickAsItsViewsDraw(t *testing.T) {
	row := func(tick uint64, x, until float64) Row {
		return Row{Tick: tick, Center: box2d.Vec2{X: x}, Radius: 0.5, Tags: interact.Tags(tick), Until: until}
	}
	type span struct{ start, end, from, to float64 } // its part of the tick, and its x from and to
	for _, c := range []struct {
		name string
		view float64
		rows []Row
		want []span // with the tags of the row each comes from: 11, then 12
	}{
		{"a whole tick's views", 10, []Row{row(10, 0, 1), row(11, 4, 1), row(12, 8, 1)}, []span{{0, 1, 0, 4}}},
		{"across a tick's end", 10.25, []Row{row(10, 0, 1), row(11, 4, 1), row(12, 8, 1)},
			[]span{{0, 0.75, 1, 4}, {0.75, 1, 4, 5}}},
		{"dying halfway through tick 11", 10.25, []Row{row(10, 0, 1), row(11, 4, 0.5)}, []span{{0, 0.25, 2, 4}}},
		{"dead before the views", 10.25, []Row{row(10, 0, 1), row(11, 4, 0.25)}, nil},
		{"new at tick 12", 10.25, []Row{row(12, 8, 1)}, []span{{}, {0.75, 1, 8, 8}}},
		{"gone at tick 12", 10.25, []Row{row(10, 0, 1), row(11, 4, 1)}, []span{{0, 0.75, 1, 4}}},
	} {
		rows := map[uint64]Row{}
		for _, r := range c.rows {
			rows[r.Tick] = r
		}
		got := Across(c.view, func(tick uint64) (Row, bool) { r, ok := rows[tick]; return r, ok })
		var want [2]Span
		for i, s := range c.want {
			if s.end > s.start {
				r := rows[uint64(11+i)]
				r.Center = box2d.Vec2{X: s.from}
				want[i] = Span{Row: r, Start: s.start, End: s.end, To: box2d.Vec2{X: s.to}}
			}
		}
		if got != want {
			t.Errorf("%s: %+v, want %+v", c.name, got, want)
		}
	}
}

// A predicted action is judged at its client's view, held to those whose tick's views are drawn from the
// rows kept by its tick: the oldest Ticks before it, judged from the oldest row kept, and the newest a
// tick before the newest row. A view that isn't a number is judged at the newest.
func TestViewIsHeldInsideTheRows(t *testing.T) {
	window := Window{InterpolationSeconds: 0.1, QueueSeconds: 0.05, PingSeconds: 0.25}
	if got, want := window.Ticks(), uint64(timestep.Ticks(0.4)); got != want {
		t.Fatalf("Ticks = %d, want %d: its parts added up", got, want)
	}
	tick := uint64(100)
	oldest := float64(tick - window.Ticks())
	newest := float64(tick - 2) // its views run to row 99, the newest
	for _, c := range []struct {
		view, want float64
	}{
		{oldest, oldest},
		{oldest - 3.5, oldest},
		{97.5, 97.5},
		{98, newest},
		{98.5, newest}, // needs row 100
		{math.NaN(), newest},
		{math.Inf(1), newest},
		{math.Inf(-1), oldest},
	} {
		if got := window.Clamp(c.view, tick); got != c.want {
			t.Errorf("Clamp(%v, %d) = %v, want %v", c.view, tick, got, c.want)
		}
	}
	read := tick
	Across(oldest, func(b uint64) (Row, bool) {
		read = min(read, b)
		return Row{Tick: b, Until: 1}, true
	})
	if read != tick-window.KeptRows() { // rows run back from tick - 1
		t.Errorf("the oldest view is judged from row %d, want the oldest kept, %d", read, tick-window.KeptRows())
	}
	if got := window.Clamp(-0.5, 3); got != 1 { // rows start at tick 0
		t.Errorf("Clamp(-0.5, 3) = %v, want 1", got)
	}
}
