package past

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
)

// Thing is one recorded thing in a scene.
type Thing struct {
	ID        uint32
	Serial    uint64
	Now       Row        // its newest row: live at the scene's tick (shard), or the newest sync's (predictor)
	Predicted bool       // its owner's client predicts it
	From, To  box2d.Vec2 // if Predicted, where its owner's screen draws it as the tick starts and ends
}

// Actor is whose action a scene judges, and the view its screen had as the tick began.
type Actor struct {
	Side      interact.Side
	View      float64 // a client's view, held by the window, or the tick before when present
	Predicted bool    // its client predicts this action
}

// Query is a shape moving over part of the tick.
type Query struct {
	Shape      box2d.ShapeProxy // around its own origin
	Start, End box2d.Vec2       // its origin's path: equal for a still shape
	From, To   float64          // the part of the tick it covers, From <= To; equal for one instant
	Kinds      interact.Kind    // what it may meet
}

// Meet is a judge's rule for a thing as placed: its rank for ties, or false to pass it.
type Meet func(r Row) (rank int, ok bool)

// Contact is a query's first touch with a thing, as its actor saw the thing.
type Contact struct {
	ID       uint32
	Serial   uint64
	Row      Row     // the thing as placed, Center where it was at the touch
	Fraction float64 // of the query's path
	Rank     int
}

// Before reports whether c comes before o: the nearer first, then the lower rank, then the lower ID.
func (c Contact) Before(o Contact) bool { return compareContacts(c, o) < 0 }

func compareContacts(a, b Contact) int {
	return cmp.Or(cmp.Compare(a.Fraction, b.Fraction), cmp.Compare(a.Rank, b.Rank), cmp.Compare(a.ID, b.ID),
		cmp.Compare(a.Serial, b.Serial))
}

// Scene is the recorded things one tick's judges meet: each one's rows over the window, one Box2D tree
// box around every spot a view can place it at, and each client's view. It is built once a tick (Reset,
// Add, AddClient, Build), then every judge queries it. It is derived, never saved, and not safe for
// concurrent use.
type Scene struct {
	window  Window
	tick    uint64
	things  []placed
	rows    []Row // each thing's rows, oldest first, its Now last
	clients []client
	tree    box2d.DynamicTree
	found   []int // a query's candidates, by index in things
}

// placed is a thing in a scene: where its rows are, and its tree box.
type placed struct {
	Thing
	first, count int
	proxy        int
}

// client is a client's view as the tick began, held by the window.
type client struct {
	id     uint32
	serial uint64
	view   float64
}

// NewScene returns an empty scene whose clients' views w holds.
func NewScene(w Window) *Scene {
	return &Scene{window: w, tree: box2d.NewDynamicTree()}
}

// Reset empties s for tick, keeping its memory.
func (s *Scene) Reset(tick uint64) {
	for _, t := range s.things {
		if t.proxy >= 0 {
			s.tree.DestroyProxy(t.proxy)
		}
	}
	s.tick, s.things, s.rows, s.clients = tick, s.things[:0], s.rows[:0], s.clients[:0]
}

// Add adds t with its kept rows, those before t.Now, oldest first.
func (s *Scene) Add(t Thing, kept []Row) {
	first := len(s.rows)
	s.rows = append(append(s.rows, kept...), t.Now)
	s.things = append(s.things, placed{Thing: t, first: first, count: len(s.rows) - first, proxy: -1})
}

// AddClient adds the client playing thing id with serial, whose screen showed view as the tick began.
func (s *Scene) AddClient(id uint32, serial uint64, view float64) {
	s.clients = append(s.clients, client{id: id, serial: serial, view: s.window.Clamp(view, s.tick)})
}

// Build sorts the clients and boxes each thing once Add is done: one tree box over its rows (and From and
// To if Predicted), padded by its largest radius. O(T·W + T log T + P log P).
func (s *Scene) Build() {
	slices.SortFunc(s.clients, func(a, b client) int { return cmp.Compare(a.id, b.id) })
	for i := range s.things {
		t := &s.things[i]
		rows := s.rows[t.first : t.first+t.count]
		lo, hi, radius := rows[0].Center, rows[0].Center, 0.0
		for _, r := range rows {
			lo, hi, radius = box2d.Min(lo, r.Center), box2d.Max(hi, r.Center), max(radius, r.Radius)
		}
		if t.Predicted {
			lo, hi = box2d.Min(box2d.Min(lo, t.From), t.To), box2d.Max(box2d.Max(hi, t.From), t.To)
		}
		pad := box2d.Vec2{X: radius, Y: radius}
		box := box2d.AABB{LowerBound: box2d.Sub(lo, pad), UpperBound: box2d.Add(hi, pad)}
		t.proxy = s.tree.CreateProxy(box, uint64(t.Now.Kind), uint64(i))
	}
}

// Client is the actor of the client playing thing id, judged at its view: false when id has no client,
// or is now another thing than the one with serial.
func (s *Scene) Client(id uint32, serial uint64, side interact.Side) (Actor, bool) {
	i, ok := slices.BinarySearchFunc(s.clients, id, func(c client, id uint32) int { return cmp.Compare(c.id, id) })
	if !ok || s.clients[i].serial != serial {
		return Actor{}, false
	}
	return Actor{Side: side, View: s.clients[i].view, Predicted: true}, true
}

// Present is an actor no client predicts: judged at this tick, as every screen draws it.
func (s *Scene) Present(side interact.Side) Actor {
	return Actor{Side: side, View: max(float64(s.tick)-1, 0)}
}

// Cast appends q's first touch with each thing meet doesn't pass, as a saw it, sorted by Before: one
// tree query of q's swept box, then a Box2D cast per part of the tick each candidate shows in. O(log T +
// K log K) for K candidates, with no allocations when out has room.
func (s *Scene) Cast(a Actor, q Query, meet Meet, out []Contact) []Contact {
	n := len(out)
	s.found = s.found[:0]
	s.tree.Query(q.box(), uint64(q.Kinds), collect, s)
	found := s.found
	s.found = nil // a meet that casts again gets its own
	for _, i := range found {
		t := &s.things[i]
		var first Contact
		met := false
		for _, sp := range s.spans(a, t) {
			if c, ok := q.cast(sp, meet); ok && (!met || c.Before(first)) {
				first, met = c, true
			}
		}
		if met {
			first.ID, first.Serial = t.ID, t.Serial
			out = append(out, first)
		}
	}
	s.found = found[:0]
	slices.SortFunc(out[n:], compareContacts)
	return out
}

// collect keeps a tree query's candidate in the scene ctx.
func collect(_ int, i uint64, ctx any) bool {
	s := ctx.(*Scene)
	s.found = append(s.found, int(i))
	return true
}

// spans is t as a sees it across the tick, by the one placement rule: from its rows across a's view and
// the next (Across), but a thing a's client predicts as its own moves as that client draws it, From to
// To.
func (s *Scene) spans(a Actor, t *placed) [2]Span {
	rows := s.rows[t.first : t.first+t.count]
	spans := Across(a.View, func(tick uint64) (Row, bool) { return rowAt(rows, tick) })
	if a.Predicted && t.Predicted && t.Now.Side.Owner == a.Side.Owner {
		for i := range spans {
			sp := &spans[i]
			sp.Center, sp.To = lerp(t.From, t.To, sp.Start), lerp(t.From, t.To, sp.End)
		}
	}
	return spans
}

// rowAt is the row of tick in rows, oldest first.
func rowAt(rows []Row, tick uint64) (Row, bool) {
	i, ok := slices.BinarySearchFunc(rows, tick, func(r Row, tick uint64) int { return cmp.Compare(r.Tick, tick) })
	if !ok {
		return Row{}, false
	}
	return rows[i], true
}

// box is the box q's shape sweeps.
func (q Query) box() box2d.AABB {
	lo, hi := q.Shape.Points[0], q.Shape.Points[0]
	for _, p := range q.Shape.Points[1:q.Shape.Count] {
		lo, hi = box2d.Min(lo, p), box2d.Max(hi, p)
	}
	r := box2d.Vec2{X: q.Shape.Radius, Y: q.Shape.Radius}
	return box2d.AABB{
		LowerBound: box2d.Add(box2d.Min(q.Start, q.End), box2d.Sub(lo, r)),
		UpperBound: box2d.Add(box2d.Max(q.Start, q.End), box2d.Add(hi, r)),
	}
}

// cast is q's first touch with a thing in span sp, from one Box2D cast in the thing's moving frame: the
// thing at the origin, q's move less the thing's. False when they don't touch, or meet passes it.
func (q Query) cast(sp Span, meet Meet) (Contact, bool) {
	if !(sp.End > sp.Start) {
		return Contact{}, false
	}
	if q.To == q.From { // one instant: the thing stays where it is then
		if q.From < sp.Start || q.From > sp.End {
			return Contact{}, false
		}
		at := lerp(sp.Center, sp.To, (q.From-sp.Start)/(sp.End-sp.Start))
		return q.castAt(sp.Row, meet, at, at, 0, 1, 1)
	}
	part := q.To - q.From
	a, b := (sp.Start-q.From)/part, (sp.End-q.From)/part // of q's path
	if !(b > a) || a >= 1 || b <= 0 {
		return Contact{}, false
	}
	center := sp.Center
	if a < 0 { // it showed before q began: from where it was then
		center, a = lerp(sp.Center, sp.To, -a/(b-a)), 0
	}
	return q.castAt(sp.Row, meet, center, sp.To, a, b, min((1-a)/(b-a), 1))
}

// castAt casts q's path from a to b against r's circle moving from center to to, up to limit of the way.
func (q Query) castAt(r Row, meet Meet, center, to box2d.Vec2, a, b, limit float64) (Contact, bool) {
	rank, ok := meet(r)
	if !ok {
		return Contact{}, false
	}
	from := lerp(q.Start, q.End, a)
	shape, offset := q.Shape, box2d.Sub(from, center)
	for i := range shape.Count {
		shape.Points[i] = box2d.Add(shape.Points[i], offset)
	}
	out := box2d.ShapeCastCircle(&box2d.Circle{Radius: r.Radius}, &box2d.ShapeCastInput{
		Proxy:       shape,
		Translation: box2d.Sub(box2d.Sub(lerp(q.Start, q.End, b), from), box2d.Sub(to, center)),
		MaxFraction: limit,
	})
	if !out.Hit {
		return Contact{}, false
	}
	r.Center = lerp(center, to, out.Fraction)
	return Contact{Row: r, Fraction: a + float64(out.Fraction*(b-a)), Rank: rank}, true
}
