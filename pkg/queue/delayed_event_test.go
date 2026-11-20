package queue_test

import (
	"cmp"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/pkg/queue"
)

// ping is an event fired on a later tick.
type ping struct {
	ID uint32
}

func (ping) Name() string { return "ping" }

// heard keeps the pings each tick fired, from the first PreUpdate system after the one that fires them.
type heard struct {
	at map[uint64][]ping
}

func (s *heard) Run(w *cardinal.World) {
	if pings := slices.Collect(w.SystemEvents[ping]()); pings != nil {
		s.at[w.TickHeight()] = pings
	}
}

// pings is a world that fires pings, with a sender in Update to schedule and cancel them.
type pings struct {
	w     *cardinal.TestWorld
	at    *sender
	heard *heard
}

func newPings(t *testing.T) *pings {
	t.Helper()

	p := &pings{at: &sender{at: map[uint64]func(*cardinal.World){}}, heard: &heard{at: map[uint64][]ping{}}}
	p.w = cardinal.NewTestWorld(t, func(w *cardinal.World) {
		queue.DeclareDelayed(w, func(a, b ping) int { return cmp.Compare(a.ID, b.ID) })
		w.RegisterSystem(p.heard, cardinal.WithHook(cardinal.PreUpdate))
		w.RegisterSystem(p.at)
	})

	return p
}

func (p *pings) tick(n int) {
	for range n {
		p.w.Tick()
	}
}

// An event fires at the start of its tick, before every system registered after DeclareDelayed. One
// scheduled for a tick that has started fires at the start of the next, before that tick's own.
func TestAnEventFiresAtTheStartOfItsTick(t *testing.T) {
	p := newPings(t)
	h := p.w.TickHeight()
	p.at.at[h] = func(w *cardinal.World) {
		queue.Schedule(w, h+3, ping{ID: 1})
		queue.Schedule(w, h+1, ping{ID: 2})
		queue.Schedule(w, h, ping{ID: 3})
	}
	p.tick(4)

	want := map[uint64][]ping{h + 1: {{ID: 3}, {ID: 2}}, h + 3: {{ID: 1}}}
	for tick := h; tick <= h+3; tick++ {
		if got := p.heard.at[tick]; !slices.Equal(got, want[tick]) {
			t.Errorf("tick %d fired %v, want %v", tick, got, want[tick])
		}
	}
}

// Events due together fire sorted by compare, whatever order they were scheduled in.
func TestEventsDueTogetherFireSorted(t *testing.T) {
	want := []ping{{ID: 1}, {ID: 2}, {ID: 3}}
	for _, order := range [][]ping{
		{want[2], want[1], want[0]},
		{want[1], want[0], want[2]},
	} {
		p := newPings(t)
		h := p.w.TickHeight()
		p.at.at[h] = func(w *cardinal.World) {
			for _, e := range order {
				queue.Schedule(w, h+1, e)
			}
		}
		p.tick(2)

		if got := p.heard.at[h+1]; !slices.Equal(got, want) {
			t.Errorf("scheduled %v: fired %v, want %v", order, got, want)
		}
	}
}

// Cancel stops a waiting event, and reports false for one that fired, was cancelled, or is zero.
// Tickets are never given out twice. CancelFunc stops every waiting event it matches.
func TestCancel(t *testing.T) {
	p := newPings(t)
	h := p.w.TickHeight()

	var soon, later, cancelled queue.Ticket[ping]

	p.at.at[h] = func(w *cardinal.World) {
		soon = queue.Schedule(w, h+1, ping{ID: 1})
		later = queue.Schedule(w, h+5, ping{ID: 2})
		cancelled = queue.Schedule(w, h+5, ping{ID: 3})
		queue.Schedule(w, h+5, ping{ID: 4})
		queue.Schedule(w, h+5, ping{ID: 5})
	}

	var (
		results []bool
		matched int
		fresh   queue.Ticket[ping]
	)

	p.at.at[h+2] = func(w *cardinal.World) {
		results = []bool{
			queue.Cancel(w, cancelled), queue.Cancel(w, cancelled), queue.Cancel(w, soon),
			queue.Cancel(w, queue.Ticket[ping](0)),
		}
		matched = queue.CancelFunc(w, func(e ping) bool { return e.ID >= 4 })
		fresh = queue.Schedule(w, h+5, ping{ID: 6})
	}
	p.tick(6)

	if want := []bool{true, false, false, false}; !slices.Equal(results, want) {
		t.Errorf("Cancel of a waiting, a cancelled, a fired and a zero ticket: %v, want %v", results, want)
	}

	if matched != 2 {
		t.Errorf("CancelFunc stopped %d, want 2", matched)
	}

	if got, want := p.heard.at[h+5], []ping{{ID: 2}, {ID: 6}}; !slices.Equal(got, want) {
		t.Errorf("tick %d fired %v, want only %v", h+5, got, want)
	}

	tickets := []queue.Ticket[ping]{soon, later, cancelled, fresh}
	if slices.Contains(tickets, 0) || len(slices.Compact(slices.Sorted(slices.Values(tickets)))) != len(tickets) {
		t.Errorf("tickets %v, want all different, none zero", tickets)
	}
}
