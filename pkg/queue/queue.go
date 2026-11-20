package queue

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// Request is a change that the owner of some state makes for another feature: a plain value struct.
// Its Name names its queue's component, queue_<name>.
type Request interface {
	Name() string
}

// waiting is a request and the tick it is due.
type waiting[R Request] struct {
	Due uint64
	Req R
}

// queue is R's queue, held by one entity. Drained is the tick it last drained, plus 1.
type queue[R Request] struct {
	Items   cardinal.Slice[waiting[R]]
	Drained uint64
}

// Name is the component name.
func (queue[R]) Name() string {
	var r R
	return "queue_" + r.Name()
}

type queueRow[R Request] struct {
	Queue queue[R]
}

// Declare registers R's queue, and an Init system that makes the entity holding it. R's owner calls it
// in its RegisterTypes.
func Declare[R Request](w *cardinal.World) {
	w.RegisterComponent[queue[R]]()
	w.RegisterSystem(&makeQueue[R]{}, cardinal.WithHook(cardinal.Init))
}

type makeQueue[R Request] struct{}

func (*makeQueue[R]) Run(w *cardinal.World) { w.Create[queueRow[R]]() }

// Send queues reqs for their owner's next drain: this tick if it hasn't drained yet, else the next.
func Send[R Request](w *cardinal.World, reqs ...R) {
	add(w, w.TickHeight(), reqs)
}

// SendAt queues reqs for their owner's first drain at or after tick.
func SendAt[R Request](w *cardinal.World, tick uint64, reqs ...R) {
	add(w, tick, reqs)
}

func add[R Request](w *cardinal.World, due uint64, reqs []R) {
	if len(reqs) == 0 {
		return
	}

	e := holder[queueRow[R]](w)

	q := e.Get[queue[R]]()
	for _, r := range reqs {
		q.Items = q.Items.Insert(q.Items.Len(), waiting[R]{Due: due, Req: r})
	}

	e.Set(q)
}

// Drain takes every request due by this tick out of R's queue. They come sorted by the tick each was
// due, then by compare, never by the order they were sent in, which can change after a restore. So
// compare must tell apart any two requests that act differently. Only R's owner drains, from one
// system, once a tick: a second drain in a tick panics.
func Drain[R Request](w *cardinal.World, compare func(a, b R) int) []R {
	now := w.TickHeight()
	e := holder[queueRow[R]](w)

	q := e.Get[queue[R]]()
	if q.Drained == now+1 {
		panic(fmt.Sprintf("queue: %s drained twice in tick %d; one owner system drains it", q.Name(), now))
	}

	q.Drained = now + 1

	var due []waiting[R]

	for x := range q.Items.Values() {
		if x.Due <= now {
			due = append(due, x)
		}
	}

	if len(due) == 0 {
		e.Set(q)
		return nil
	}

	q.Items = q.Items.Filter(func(x waiting[R]) bool { return x.Due > now })
	e.Set(q)
	slices.SortFunc(due, func(a, b waiting[R]) int {
		return cmp.Or(cmp.Compare(a.Due, b.Due), compare(a.Req, b.Req))
	})

	reqs := make([]R, len(due))
	for i, x := range due {
		reqs[i] = x.Req
	}

	return reqs
}

// holder is the one entity a queue lives on.
func holder[Row any](w *cardinal.World) cardinal.Entity {
	e, err := w.Exact[Row]().Iter().Single()
	if err != nil {
		var row Row
		panic(fmt.Sprintf("queue: the entity holding %T: %v; declare it in its owner's RegisterTypes, "+
			"or purge a world saved before it was declared", row, err))
	}

	return e
}
