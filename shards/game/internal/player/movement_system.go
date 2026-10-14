package player

import (
	"cmp"
	"math"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/player/component"
)

// movableRow is a player Body we can drive.
type movableRow struct {
	Player   component.Player
	Input    component.Input
	Speed    component.MoveSpeed
	Velocity physics2d.Velocity2D
}

// MovementSystem (PreUpdate, before the physics step, so a frame moves the Body the same tick it is
// acked) queues each Move's frames on the sender's Body, then applies exactly one queued frame per
// Body per tick, in Seq order. One step per tick keeps the Body, and the Echo replaying it, smooth
// when frames arrive late or bunched. A Body with no frame keeps its velocity until the input timeout,
// then stops. It is the only writer of Input and of a Body's Velocity2D, and emits BodyClaimed the
// first tick each Body is claimed.
type MovementSystem struct {
	Data *data.Plugin
}

func (s *MovementSystem) Run(w *cardinal.World) {
	tuning := data.Get[MovementTuning](s.Data)
	movables := w.Contains[movableRow]()
	receive(w, movables, tuning.BufferFrames())
	apply(w, movables, tuning.TimeoutTicks())
}

// receive queues every Move this tick on its sender's Body. O(B + C): Bodies are indexed once.
func receive(w *cardinal.World, movables cardinal.Search, limit int) {
	var index *bodyIndex
	for cmd := range w.Commands[Move]() {
		player, ok := cmd.Player()
		if !ok || player == "" {
			// An empty player ID would claim a fresh Body every tick. Only players move Bodies.
			w.Logger().Warn().Str("sender", cmd.Sender()).Msg("move: not from a player, ignoring command")
			continue
		}
		if index == nil {
			index = indexBodies(movables)
		}
		body, ok := index.resolve(w, player)
		if !ok {
			w.Logger().Warn().Str("player", player).Msg("move: no free player Body to control")
			continue
		}
		body.Set(enqueue(body.Get[component.Input](), cmd.Payload, limit))
	}
}

// enqueue merges m's frames into in.Pending in Seq order, skipping applied and duplicate frames, and
// keeps only the newest limit frames. A new Session starts the stream over.
func enqueue(in component.Input, m Move, limit int) component.Input {
	if m.Session != in.Session {
		in = component.Input{Session: m.Session, Tick: in.Tick}
	}
	pending := slices.Collect(in.Pending.Values())
	for f := range m.Frames.Values() {
		if f.Seq <= in.Ack {
			continue
		}
		i, found := slices.BinarySearchFunc(pending, f.Seq, func(p component.MoveFrame, seq uint32) int {
			return cmp.Compare(p.Seq, seq)
		})
		if !found {
			pending = slices.Insert(pending, i, f)
		}
	}
	if len(pending) > limit {
		pending = pending[len(pending)-limit:]
	}
	in.Pending = cardinal.SliceOf(pending...)
	return in
}

// apply moves each Body by its next queued frame. O(B).
func apply(w *cardinal.World, movables cardinal.Search, timeout uint64) {
	tick := w.TickHeight()
	for body := range movables.Iter() {
		in := body.Get[component.Input]()
		if in.Pending.Len() == 0 {
			// No frame this tick: keep moving through a late frame, but stop a client that went silent.
			if tick > in.Tick+timeout && body.Get[physics2d.Velocity2D]() != (physics2d.Velocity2D{}) {
				body.Set(physics2d.Velocity2D{})
			}
			continue
		}
		f := in.Pending.At(0)
		in.Pending = in.Pending.Sub(1, in.Pending.Len())
		in.Ack, in.Tick = f.Seq, tick
		body.Set(in)
		x, y := normalize(f.X, f.Y)
		speed := body.Get[component.MoveSpeed]().Value
		body.Set(physics2d.Velocity2D{Linear: physics2d.Vec2{X: x * speed, Y: y * speed}})
	}
}

// bodyIndex finds a player's Body: the one they own, or else the next unclaimed one. Claim on first
// command is dev-spawn scaffolding; matchmaking will assign owners at spawn.
type bodyIndex struct {
	owned map[string]cardinal.Entity
	free  []cardinal.Entity
}

func indexBodies(movables cardinal.Search) *bodyIndex {
	ix := &bodyIndex{owned: map[string]cardinal.Entity{}}
	for e := range movables.Iter() {
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

// claim makes player the Body's owner and edge-triggers echo.SpawnSystem. A Body is claimed once
// (Persona is never cleared), so it gets exactly one Echo.
func claim(w *cardinal.World, e cardinal.Entity, player string) {
	p := e.Get[component.Player]()
	p.Persona = player
	e.Set(p)
	w.EmitSystemEvent(BodyClaimed{Body: e.ID()})
	w.Logger().Info().Str("player", player).Uint32("id", p.ID).Msg("player Body claimed")
}

// normalize returns the unit vector of (x, y), or (0, 0) for a ~zero or non-finite input, so a bad
// client frame can never put NaN into physics.
func normalize(x, y float64) (float64, float64) {
	mag := math.Hypot(x, y)
	if mag < 1e-9 || math.IsInf(mag, 0) || math.IsNaN(mag) {
		return 0, 0
	}
	return x / mag, y / mag
}
