package queue

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// Event is a system event that can fire on a later tick. Like a Request, it is a plain value struct.
type Event = Request

// Ticket names one scheduled event, so it can be cancelled. Zero names none.
type Ticket[E Event] uint64

// scheduled is an event, the tick it fires and its ticket.
type scheduled[E Event] struct {
	Due    uint64
	Ticket uint64
	Event  E
}

// delayed is E's scheduled events, held by one entity. Last is the last ticket given out, so none is
// given out twice, even after a restore.
type delayed[E Event] struct {
	Items cardinal.Slice[scheduled[E]]
	Last  uint64
}

// Name is the component name.
func (delayed[E]) Name() string {
	var e E
	return "delayed_" + e.Name()
}

type delayedRow[E Event] struct {
	Delayed delayed[E]
}

// DeclareDelayed registers the system event E, its delayed queue, an Init system that makes the entity
// holding it, and the PreUpdate system that fires it. That system runs where DeclareDelayed is called,
// so call it before the systems that read E: each then sees an E the tick it fires. compare orders the
// events due in one tick, so it must tell apart any two that act differently.
func DeclareDelayed[E Event](w *cardinal.World, compare func(a, b E) int) {
	w.RegisterSystemEvent[E]()
	w.RegisterComponent[delayed[E]]()
	w.RegisterSystem(&makeDelayed[E]{}, cardinal.WithHook(cardinal.Init))
	w.RegisterSystem(&fire[E]{compare: compare}, cardinal.WithHook(cardinal.PreUpdate))
}

type makeDelayed[E Event] struct{}

func (*makeDelayed[E]) Run(w *cardinal.World) { w.Create[delayedRow[E]]() }

// Schedule fires e at the start of tick, or at the next start if that one has passed. It returns the
// ticket that cancels it.
func Schedule[E Event](w *cardinal.World, tick uint64, e E) Ticket[E] {
	h := holder[delayedRow[E]](w)
	d := h.Get[delayed[E]]()
	d.Last++
	d.Items = d.Items.Insert(d.Items.Len(), scheduled[E]{Due: tick, Ticket: d.Last, Event: e})
	h.Set(d)

	return Ticket[E](d.Last)
}

// Cancel stops the event t names. It reports false if that event already fired or was cancelled, or t
// is zero.
func Cancel[E Event](w *cardinal.World, t Ticket[E]) bool {
	h := holder[delayedRow[E]](w)
	d := h.Get[delayed[E]]()

	i := d.Items.IndexFunc(func(x scheduled[E]) bool { return x.Ticket == uint64(t) })
	if i < 0 {
		return false
	}

	d.Items = d.Items.Without(i)
	h.Set(d)

	return true
}

// CancelFunc stops every waiting event that match reports true for, and returns how many it stopped.
func CancelFunc[E Event](w *cardinal.World, match func(E) bool) int {
	h := holder[delayedRow[E]](w)

	d := h.Get[delayed[E]]()
	if !d.Items.ContainsFunc(func(x scheduled[E]) bool { return match(x.Event) }) {
		return 0
	}

	before := d.Items.Len()
	d.Items = d.Items.Filter(func(x scheduled[E]) bool { return !match(x.Event) })
	h.Set(d)

	return before - d.Items.Len()
}

// fire emits E's events due this tick, sorted by the tick each was due, then compare.
type fire[E Event] struct {
	compare func(a, b E) int
}

func (s *fire[E]) Run(w *cardinal.World) {
	now := w.TickHeight()
	h := holder[delayedRow[E]](w)
	d := h.Get[delayed[E]]()

	var due []scheduled[E]

	for x := range d.Items.Values() {
		if x.Due <= now {
			due = append(due, x)
		}
	}

	if len(due) == 0 {
		return
	}

	d.Items = d.Items.Filter(func(x scheduled[E]) bool { return x.Due > now })
	h.Set(d)
	slices.SortFunc(due, func(a, b scheduled[E]) int {
		return cmp.Or(cmp.Compare(a.Due, b.Due), s.compare(a.Event, b.Event), cmp.Compare(a.Ticket, b.Ticket))
	})

	for _, x := range due {
		w.EmitSystemEvent(x.Event)
	}
}
