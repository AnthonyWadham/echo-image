package player

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"

	"echo_image/shards/game/internal/player/component"
)

// inputRow is a player Body that takes input.
type inputRow struct {
	Player component.Player
	Input  component.InputQueue
}

// InputSystem (PreUpdate, first) queues each Input command's frames on the sender's Body, then
// applies exactly one queued frame per Body per tick, in Seq order. One frame per tick keeps the Body,
// and the Echo replaying it, smooth when frames arrive late or bunched. Systems act on the applied
// frame through InputQueue.Applied. It is the only writer of InputQueue, and emits BodyClaimed the
// first tick each Body is claimed.
type InputSystem struct {
	Data *data.Plugin
}

func (s *InputSystem) Run(w *cardinal.World) {
	limit := data.Get[MovementTuning](s.Data).BufferFrames()
	bodies := w.Contains[inputRow]()
	receive(w, bodies, limit)
	advance(w, bodies)
}

// receive queues every Input this tick on its sender's Body. O(B + C): Bodies are indexed once.
func receive(w *cardinal.World, bodies cardinal.Search, limit int) {
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
		body, ok := index.resolve(w, player)
		if !ok {
			w.Logger().Warn().Str("player", player).Msg("input: no free player Body to control")
			continue
		}
		body.Set(enqueue(body.Get[component.InputQueue](), cmd.Payload, limit))
	}
}

// enqueue merges in's frames into q.Pending in Seq order, skipping applied and duplicate frames, and
// keeps only the newest limit frames. A new Session starts the stream over.
func enqueue(q component.InputQueue, in Input, limit int) component.InputQueue {
	if in.Session != q.Session {
		q = component.InputQueue{Session: in.Session, LastTick: q.LastTick}
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
		pending = pending[len(pending)-limit:]
	}
	q.Pending = cardinal.SliceOf(pending...)
	return q
}

// advance applies each Body's next queued frame. O(B).
func advance(w *cardinal.World, bodies cardinal.Search) {
	tick := w.TickHeight()
	for body := range bodies.Iter() {
		q := body.Get[component.InputQueue]()
		if q.Pending.Len() == 0 {
			continue
		}
		q.Last, q.LastTick = q.Pending.At(0), tick
		q.Pending = q.Pending.Sub(1, q.Pending.Len())
		body.Set(q)
	}
}

// bodyIndex finds a player's Body: the one they own, or else the next unclaimed one. Claim on first
// command is dev-spawn scaffolding; matchmaking will assign owners at spawn.
type bodyIndex struct {
	owned map[string]cardinal.Entity
	free  []cardinal.Entity
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
