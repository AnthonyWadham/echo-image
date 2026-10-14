package main

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/netsync"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/team"
)

func move(session uint32, frames ...playercomp.MoveFrame) player.Move {
	return player.Move{Session: session, Frames: cardinal.SliceOf(frames...)}
}

func frame(seq uint32, x, y float64) playercomp.MoveFrame {
	return playercomp.MoveFrame{Seq: seq, X: x, Y: y}
}

// tuning reads the shipped movement tuning, so tests follow the real buffer and timeout.
func tuning(t *testing.T) player.MovementTuning {
	t.Helper()
	raw, err := content.FS.ReadFile(player.MovementTuning{}.JSONFile())
	if err != nil {
		t.Fatal(err)
	}
	var mt player.MovementTuning
	if err := json.Unmarshal(raw, &mt); err != nil {
		t.Fatal(err)
	}
	return mt
}

// stateFor returns the WorldState the last tick sent to player, failing if there isn't exactly one.
func stateFor(t *testing.T, w *cardinal.TestWorld, playerID string) netsync.WorldState {
	t.Helper()
	var found []netsync.WorldState
	for _, sent := range w.Events[netsync.WorldState]() {
		if sent.Recipient == "" {
			t.Fatal("world_state was broadcast; it must be sent per player")
		}
		if sent.Recipient == playerID {
			found = append(found, sent.Payload)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want 1 world_state for %s, got %d", playerID, len(found))
	}
	return found[0]
}

func requireAck(t *testing.T, s netsync.WorldState, want uint32) {
	t.Helper()
	if s.You.InputAck != want {
		t.Fatalf("InputAck = %d, want %d", s.You.InputAck, want)
	}
}

// yourBody returns the position of the receiving player's own Body.
func yourBody(t *testing.T, s netsync.WorldState) (x, y float64) {
	t.Helper()
	for _, b := range s.Bodies.All() {
		if b.EntityID == s.You.Body {
			return b.X, b.Y
		}
	}
	t.Fatalf("your Body %d is not in the state", s.You.Body)
	return 0, 0
}

// requireMoved fails unless the Body moved exactly one tick of full-speed input along (dx, dy).
func requireMoved(t *testing.T, before, after netsync.WorldState, dx, dy float64) {
	t.Helper()
	x0, y0 := yourBody(t, before)
	x1, y1 := yourBody(t, after)
	step := after.You.MoveSpeed * after.TickSeconds
	const eps = 1e-4
	// Written as !(within) so a NaN position fails too.
	if !(math.Abs(x1-x0-dx*step) <= eps && math.Abs(y1-y0-dy*step) <= eps) {
		t.Fatalf("moved (%.4f, %.4f), want (%.4f, %.4f)", x1-x0, y1-y0, dx*step, dy*step)
	}
}

// The client predicts by replaying frames newer than InputAck, one tick of movement each. That only
// works if an acked frame has already moved the Body in the position sent with the ack.
func TestFrameMovesBodySameTickAsAck(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)

	w.Command("alice", move(1, frame(1, 1, 0)))
	w.Tick()
	first := stateFor(t, w, "alice")
	requireAck(t, first, 1)

	// A new direction must show up in this tick's position, not the next one.
	w.Command("alice", move(1, frame(2, 0, 1)))
	w.Tick()
	second := stateFor(t, w, "alice")
	requireAck(t, second, 2)
	requireMoved(t, first, second, 0, 1)

	// No frame this tick: the Body keeps moving through a late frame and the ack stays.
	w.Tick()
	third := stateFor(t, w, "alice")
	requireAck(t, third, 2)
	requireMoved(t, second, third, 0, 1)
}

func TestFramesApplyOnePerTickInSeqOrder(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", move(1, frame(1, 0, 0)))
	w.Tick()
	start := stateFor(t, w, "alice")

	// Frames 2 and 3 arrive bunched and out of order: neither is dropped, and they apply in Seq order.
	w.Command("alice", move(1, frame(3, 0, 1)))
	w.Command("alice", move(1, frame(2, 1, 0)))
	w.Tick()
	a := stateFor(t, w, "alice")
	requireAck(t, a, 2)
	requireMoved(t, start, a, 1, 0)

	w.Tick()
	b := stateFor(t, w, "alice")
	requireAck(t, b, 3)
	requireMoved(t, a, b, 0, 1)
}

func TestResentAndStaleFramesAreIgnored(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", move(1, frame(1, 1, 0), frame(2, 1, 0)))
	w.Tick()
	w.Tick()
	before := stateFor(t, w, "alice")
	requireAck(t, before, 2)

	// A late message resends frames 1 and 2. Both were applied, so the ack never goes backwards.
	w.Command("alice", move(1, frame(1, -1, 0), frame(2, -1, 0)))
	w.Tick()
	after := stateFor(t, w, "alice")
	requireAck(t, after, 2)
	requireMoved(t, before, after, 1, 0)
}

func TestNewSessionStartsOver(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", move(1, frame(5, 1, 0)))
	w.Tick()
	before := stateFor(t, w, "alice")

	// A restarted client counts from 1 again under a new session.
	w.Command("alice", move(2, frame(1, -1, 0)))
	w.Tick()
	after := stateFor(t, w, "alice")
	if after.You.InputSession != 2 {
		t.Fatalf("InputSession = %d, want 2", after.You.InputSession)
	}
	requireAck(t, after, 1)
	requireMoved(t, before, after, -1, 0)
}

func TestBufferKeepsNewestFrames(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	limit := uint32(tuning(t).BufferFrames())

	// After a stall, 10 frames arrive at once. Only the newest `limit` are kept, so latency stays bounded.
	var frames []playercomp.MoveFrame
	for seq := uint32(1); seq <= 10; seq++ {
		frames = append(frames, frame(seq, 1, 0))
	}
	w.Command("alice", move(1, frames...))
	w.Tick()
	requireAck(t, stateFor(t, w, "alice"), 10-limit+1)
	for range limit - 1 {
		w.Tick()
	}
	requireAck(t, stateFor(t, w, "alice"), 10)
}

func TestSilentClientStops(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", move(1, frame(1, 1, 0)))
	w.Tick()

	// The client goes silent while moving (alt-tab, crash). After the timeout its Body stops.
	for range tuning(t).TimeoutTicks() + 1 {
		w.Tick()
	}
	before := stateFor(t, w, "alice")
	w.Tick()
	requireMoved(t, before, stateFor(t, w, "alice"), 0, 0)
}

func TestBadFrameDoesNotMove(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	// Start moving: Box2D won't wake a sleeping Body for a NaN velocity, which would hide the bug.
	w.Command("alice", move(1, frame(1, 1, 0)))
	w.Tick()
	before := stateFor(t, w, "alice")

	// A bad frame reads as "no input": the Body stops instead of taking NaN into physics.
	w.Command("alice", move(1, frame(2, math.NaN(), math.Inf(1))))
	w.Tick()
	requireMoved(t, before, stateFor(t, w, "alice"), 0, 0)
}

func TestWorldStateOnlyGoesToPlayersWithABody(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Tick()
	if sent := w.Events[netsync.WorldState](); len(sent) != 0 {
		t.Fatalf("sent %d world_state events before anyone took a Body", len(sent))
	}

	w.Command("alice", move(1, frame(1, 0, 0)))
	w.Command("bob", move(1, frame(1, 0, 0)))
	w.Tick()
	a, b := stateFor(t, w, "alice"), stateFor(t, w, "bob")
	if a.You.Body == b.You.Body {
		t.Fatal("alice and bob got the same Body")
	}
	if a.Bodies.Len() != 2 || a.Echoes.Len() != 2 {
		t.Fatalf("alice sees %d Bodies and %d Echoes, want 2 and 2", a.Bodies.Len(), a.Echoes.Len())
	}
}

// netsync must not depend on movement internals: a Body without them is still shown to its owner.
func TestBodyWithoutMovementIsStillShown(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	type shownRow struct {
		Player    playercomp.Player
		Team      team.Team
		Transform physics2d.Transform2D
	}
	e := w.Create[shownRow]()
	e.Set(playercomp.Player{ID: 9, Persona: "carol", Nickname: "Carol"})
	w.Tick()

	s := stateFor(t, w, "carol")
	if s.You.Body != uint32(e.ID()) || s.You.InputAck != 0 || s.You.MoveSpeed != 0 {
		t.Fatalf("You = %+v, want Body %d with zero ack and speed", s.You, e.ID())
	}
}
