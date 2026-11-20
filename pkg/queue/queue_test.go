package queue_test

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/pkg/queue"
)

// hit asks its owner to take Amount from Target.
type hit struct {
	Target uint32
	Amount int
}

func (hit) Name() string { return "hit" }

func compareHits(a, b hit) int {
	return cmp.Or(cmp.Compare(a.Target, b.Target), cmp.Compare(a.Amount, b.Amount))
}

// heal is a second kind of request, so two queues share each test world.
type heal struct {
	Target uint32
}

func (heal) Name() string { return "heal" }

// sender runs each tick's sends: at[h] at tick height h.
type sender struct {
	at map[uint64]func(w *cardinal.World)
}

func (s *sender) Run(w *cardinal.World) {
	if send := s.at[w.TickHeight()]; send != nil {
		send(w)
	}
}

// owner drains hits and heals once a tick, and keeps what each tick drained.
type owner struct {
	hits  map[uint64][]hit
	heals map[uint64]int
}

func (o *owner) Run(w *cardinal.World) {
	if hits := queue.Drain(w, compareHits); hits != nil {
		o.hits[w.TickHeight()] = hits
	}

	if heals := queue.Drain(w, func(a, b heal) int { return cmp.Compare(a.Target, b.Target) }); heals != nil {
		o.heals[w.TickHeight()] = len(heals)
	}
}

// queues is a world with one sender before the owner and one after, each with what it sends by tick.
type queues struct {
	w             *cardinal.TestWorld
	before, after *sender
	owner         *owner
}

func newQueues(t *testing.T) *queues {
	t.Helper()

	q := &queues{
		before: &sender{at: map[uint64]func(*cardinal.World){}},
		after:  &sender{at: map[uint64]func(*cardinal.World){}},
		owner:  &owner{hits: map[uint64][]hit{}, heals: map[uint64]int{}},
	}
	q.w = cardinal.NewTestWorld(t, func(w *cardinal.World) {
		queue.Declare[hit](w)
		queue.Declare[heal](w)
		w.RegisterSystem(q.before)
		w.RegisterSystem(q.owner)
		w.RegisterSystem(q.after)
	})

	return q
}

func (q *queues) tick(n int) {
	for range n {
		q.w.Tick()
	}
}

// A request sent before its owner drains lands that tick. One sent after waits for the next drain: it
// is never lost.
func TestARequestLandsAtTheOwnersNextDrain(t *testing.T) {
	q := newQueues(t)
	h := q.w.TickHeight()
	q.before.at[h] = func(w *cardinal.World) { queue.Send(w, hit{Target: 1, Amount: 5}) }
	q.after.at[h] = func(w *cardinal.World) {
		queue.Send(w, hit{Target: 2, Amount: 7})
		queue.Send(w, heal{Target: 2})
	}
	q.tick(2)

	if got, want := q.owner.hits[h], []hit{{Target: 1, Amount: 5}}; !slices.Equal(got, want) {
		t.Errorf("tick %d drained %v, want %v", h, got, want)
	}

	if got, want := q.owner.hits[h+1], []hit{{Target: 2, Amount: 7}}; !slices.Equal(got, want) {
		t.Errorf("tick %d drained %v, want the late %v", h+1, got, want)
	}

	if q.owner.heals[h] != 0 || q.owner.heals[h+1] != 1 {
		t.Errorf("heals drained %v, want 1 at tick %d", q.owner.heals, h+1)
	}
}

// Drain sorts by the tick each request was due, then the owner's compare, so the order requests were
// sent in never counts.
func TestDrainSortsBySendOrderNever(t *testing.T) {
	late := hit{Target: 9, Amount: 1} // due a tick early: sent after the drain
	first := hit{Target: 1, Amount: 1}
	second := hit{Target: 1, Amount: 3}
	third := hit{Target: 2, Amount: 1}

	want := []hit{late, first, second, third}
	for _, order := range [][]hit{
		{first, second, third},
		{third, second, first},
		{second, third, first},
	} {
		q := newQueues(t)
		h := q.w.TickHeight()
		q.after.at[h] = func(w *cardinal.World) { queue.Send(w, late) }
		q.before.at[h+1] = func(w *cardinal.World) { queue.Send(w, order...) }
		q.tick(2)

		if got := q.owner.hits[h+1]; !slices.Equal(got, want) {
			t.Errorf("sent %v: drained %v, want %v", order, got, want)
		}
	}
}

// A request sent for a later tick waits for that tick's drain; one sent for a past tick lands now,
// before now's own.
func TestSendAtWaitsForItsTick(t *testing.T) {
	q := newQueues(t)
	h := q.w.TickHeight()
	q.before.at[h+1] = func(w *cardinal.World) {
		queue.SendAt(w, h+4, hit{Target: 1})
		queue.Send(w, hit{Target: 2})
		queue.SendAt(w, h, hit{Target: 3})
	}
	q.tick(5)

	if got, want := q.owner.hits[h+1], []hit{{Target: 3}, {Target: 2}}; !slices.Equal(got, want) {
		t.Errorf("tick %d drained %v, want the past tick's first: %v", h+1, got, want)
	}

	for tick := h + 2; tick < h+4; tick++ {
		if got := q.owner.hits[tick]; got != nil {
			t.Errorf("tick %d drained %v, want nothing yet", tick, got)
		}
	}

	if got, want := q.owner.hits[h+4], []hit{{Target: 1}}; !slices.Equal(got, want) {
		t.Errorf("tick %d drained %v, want %v", h+4, got, want)
	}
}

// twice drains hits twice in one tick.
type twice struct{}

func (twice) Run(w *cardinal.World) {
	queue.Drain(w, compareHits)
	queue.Drain(w, compareHits)
}

// One owner system drains a queue, once a tick: a second drain in a tick panics.
func TestASecondDrainInATickPanics(t *testing.T) {
	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		queue.Declare[hit](w)
		w.RegisterSystem(twice{})
	})

	defer func() {
		if r, ok := recover().(string); !ok || !strings.Contains(r, "queue_hit drained twice") {
			t.Errorf("recovered %v, want a panic that names queue_hit", r)
		}
	}()

	w.Tick()
}
