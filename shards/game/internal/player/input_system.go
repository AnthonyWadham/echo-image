package player

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/player/component"
)

// inputRow is a player Body that takes input.
type inputRow struct {
	Player component.Player
	Input  component.InputQueue
}

// InputSystem (PreUpdate, first) queues each Input command's frames on the sender's Body, then
// applies exactly one queued frame per Body per tick, in Seq order. One frame per tick keeps the Body,
// and the Echo replaying it, smooth when frames arrive late or bunched. The client paces its frames by
// the queue's depth, which YouState sends, so the queue's cap only stops abuse. Systems act on the
// applied frame through InputQueue.Applied. It is the only writer of InputQueue, and emits BodyClaimed
// the first tick each Body is claimed.
type InputSystem struct {
	Tuning  MovementTuning
	Metrics InputMetrics
}

func (s *InputSystem) Run(w *cardinal.World) {
	bodies := w.Contains[inputRow]()
	s.receive(w, bodies)
	s.advance(w, bodies)
}

// receive queues every Input this tick on its sender's Body. O(B + C·L²) for C commands and a queue of L
// frames: Bodies are indexed once.
func (s *InputSystem) receive(w *cardinal.World, bodies cardinal.Search) {
	limit := s.Tuning.BufferFrames()
	var index *bodyIndex
	for cmd := range w.Commands[Input]() {
		player, ok := cmd.Player()
		if !ok || player == "" {
			// An empty player ID would claim a fresh Body every tick. Only players drive Bodies.
			w.Logger().Warn().Str("sender", cmd.Sender()).Msg("input: not from a player, ignoring command")
			continue
		}
		if index == nil {
			index = indexBodies(bodies)
		}
		if cmd.Payload.Frames.Len() > limit {
			// YouState.InputCap tells clients the limit, so only abuse sends more, and merging it costs O(n²).
			// Counted, not logged, so a bad client can't flood the logs.
			s.Metrics.oversized.Add(w.Context(), 1)
			if body, ok := index.owned[player]; ok {
				q := body.Get[component.InputQueue]()
				q.Health.OversizedInputs++
				body.Set(q)
			}
			continue
		}
		body, ok := index.resolve(w, player)
		if !ok {
			w.Logger().Warn().Str("player", player).Msg("input: no free player Body to control")
			continue
		}
		q := body.Get[component.InputQueue]()
		next := enqueue(q, cmd.Payload, limit)
		if dropped := next.Health.DroppedFrames - q.Health.DroppedFrames; dropped > 0 {
			s.Metrics.dropped.Add(w.Context(), int64(dropped))
		}
		body.Set(next)
	}
}

// enqueue merges in's frames into q.Pending in Seq order, skipping applied and duplicate frames, and
// keeps only the newest limit frames. A new Session starts the stream over. O(limit²): receive drops an
// Input with more frames.
func enqueue(q component.InputQueue, in Input, limit int) component.InputQueue {
	if in.Session != q.Session {
		q.Session, q.Pending, q.Last = in.Session, cardinal.Slice[component.InputFrame]{}, component.InputFrame{}
	}
	pending := slices.Collect(q.Pending.Values())
	for f := range in.Frames.Values() {
		if f.Seq <= q.Ack() {
			continue
		}
		i, found := slices.BinarySearchFunc(pending, f.Seq, func(p component.InputFrame, seq uint32) int {
			return cmp.Compare(p.Seq, seq)
		})
		if !found {
			pending = slices.Insert(pending, i, f)
		}
	}
	if len(pending) > limit {
		q.Health.DroppedFrames += uint32(len(pending) - limit)
		pending = pending[len(pending)-limit:]
	}
	q.Pending = cardinal.SliceOf(pending...)
	return q
}

// advance applies each Body's next queued frame, its view no older than the largest so far. A queue
// that runs dry while its client sends raises the client's target: a coast is a correction. O(B).
func (s *InputSystem) advance(w *cardinal.World, bodies cardinal.Search) {
	tick, ctx, t := w.TickHeight(), w.Context(), s.Tuning
	for body := range bodies.Iter() {
		q := body.Get[component.InputQueue]()
		if q.Pending.Len() == 0 {
			if q.Last.Seq != 0 && q.LastTick == tick-1 { // it just ran dry
				body.Set(q.Coast(tick, t.QueueStepFrames(), t.QueueCalmTicks(), t.BufferFrames()))
				s.Metrics.coasts.Add(ctx, 1)
			}
			continue
		}
		q.Last, q.LastTick = q.Pending.At(0), tick
		if !(q.Last.ViewTick >= q.ViewFloor) { // NaN too
			q.Last.ViewTick = q.ViewFloor
			q.Health.FlooredViews++
			s.Metrics.floored.Add(ctx, 1)
		}
		q.ViewFloor = q.Last.ViewTick
		q.Pending = q.Pending.Sub(1, q.Pending.Len())
		s.Metrics.queued.Record(ctx, int64(q.Pending.Len()))
		body.Set(q)
	}
}

// bodyIndex finds a player's Body: the one they own, or else the unclaimed one with the lowest Player
// ID, so players claim in the dev roster's order whatever order storage holds them in. Claim on first
// command is dev-spawn scaffolding; matchmaking will assign owners at spawn.
type bodyIndex struct {
	owned map[string]cardinal.Entity
	free  []cardinal.Entity // by Player ID
}

func indexBodies(bodies cardinal.Search) *bodyIndex {
	ix := &bodyIndex{owned: map[string]cardinal.Entity{}}
	for e := range bodies.Iter() {
		if owner := e.Get[component.Player]().Persona; owner != "" {
			ix.owned[owner] = e
		} else {
			ix.free = append(ix.free, e)
		}
	}
	slices.SortFunc(ix.free, func(a, b cardinal.Entity) int {
		return cmp.Compare(a.Get[component.Player]().ID, b.Get[component.Player]().ID)
	})
	return ix
}

// resolve returns player's Body, claiming a free one on their first command.
func (ix *bodyIndex) resolve(w *cardinal.World, player string) (cardinal.Entity, bool) {
	if e, ok := ix.owned[player]; ok {
		return e, true
	}
	if len(ix.free) == 0 {
		return cardinal.Entity{}, false
	}
	e := ix.free[0]
	ix.free = ix.free[1:]
	ix.owned[player] = e
	claim(w, e, player)
	return e, true
}

// claim makes player the Body's owner and emits BodyClaimed. A Body is claimed once (Persona is never
// cleared), so it is set up once.
func claim(w *cardinal.World, e cardinal.Entity, player string) {
	p := e.Get[component.Player]()
	p.Persona = player
	e.Set(p)
	w.EmitSystemEvent(component.BodyClaimed{Body: e.ID()})
	w.Logger().Info().Str("player", player).Uint32("id", p.ID).Msg("player Body claimed")
}
