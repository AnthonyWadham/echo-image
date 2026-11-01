package main

import (
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/echo"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/projectile"
	"math"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

func requireAck(t *testing.T, s player.BodySync, want uint32) {
	t.Helper()
	if s.You.InputAck != want {
		t.Fatalf("InputAck = %d, want %d", s.You.InputAck, want)
	}
}

// The client predicts by replaying frames newer than InputAck, one tick of movement each. That only
// works if an acked frame has already moved the Body in the position sent with the ack.
func TestFrameMovesBodySameTickAsAck(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)

	w.Command("alice", input(1, frame(1, 1, 0)))
	w.Tick()
	first := bodiesFor(t, w, "alice")
	requireAck(t, first, 1)

	// A new direction must show up in this tick's position, not the next one.
	w.Command("alice", input(1, frame(2, 0, 1)))
	w.Tick()
	second := bodiesFor(t, w, "alice")
	requireAck(t, second, 2)
	requireMoved(t, first, second, 0, 1)

	// No frame this tick: the Body keeps moving through a late frame and the ack stays.
	w.Tick()
	third := bodiesFor(t, w, "alice")
	requireAck(t, third, 2)
	requireMoved(t, second, third, 0, 1)
}

func TestFramesApplyOnePerTickInSeqOrder(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", input(1, frame(1, 0, 0)))
	w.Tick()
	start := bodiesFor(t, w, "alice")

	// Frames 2 and 3 arrive bunched and out of order: neither is dropped, and they apply in Seq order.
	// The client is told one waits, so it can pace its frames.
	w.Command("alice", input(1, frame(3, 0, 1)))
	w.Command("alice", input(1, frame(2, 1, 0)))
	w.Tick()
	a := bodiesFor(t, w, "alice")
	requireAck(t, a, 2)
	requireMoved(t, start, a, 1, 0)
	if a.You.InputQueued != 1 {
		t.Fatalf("InputQueued = %d with frame 3 waiting, want 1", a.You.InputQueued)
	}

	w.Tick()
	b := bodiesFor(t, w, "alice")
	requireAck(t, b, 3)
	requireMoved(t, a, b, 0, 1)
	if b.You.InputQueued != 0 {
		t.Fatalf("InputQueued = %d with none waiting, want 0", b.You.InputQueued)
	}
}

func TestResentAndStaleFramesAreIgnored(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", input(1, frame(1, 1, 0), frame(2, 1, 0)))
	w.Tick()
	w.Tick()
	before := bodiesFor(t, w, "alice")
	requireAck(t, before, 2)

	// A late message resends frames 1 and 2. Both were applied, so the ack never goes backwards.
	w.Command("alice", input(1, frame(1, -1, 0), frame(2, -1, 0)))
	w.Tick()
	after := bodiesFor(t, w, "alice")
	requireAck(t, after, 2)
	requireMoved(t, before, after, 1, 0)
}

func TestNewSessionStartsOver(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", input(1, frame(5, 1, 0)))
	w.Tick()
	before := bodiesFor(t, w, "alice")

	// A restarted client counts from 1 again under a new session.
	w.Command("alice", input(2, frame(1, -1, 0)))
	w.Tick()
	after := bodiesFor(t, w, "alice")
	if after.You.InputSession != 2 {
		t.Fatalf("InputSession = %d, want 2", after.You.InputSession)
	}
	requireAck(t, after, 1)
	requireMoved(t, before, after, -1, 0)
}

func TestBufferKeepsNewestFrames(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	limit := uint32(tuningOf[player.MovementTuning](t).BufferFrames())
	frames := func(from, to uint32) []playercomp.InputFrame {
		var fs []playercomp.InputFrame
		for seq := from; seq <= to; seq++ {
			fs = append(fs, frame(seq, 1, 0))
		}
		return fs
	}

	// An Input with more frames than the queue holds is dropped whole, before it claims a Body: YouState
	// tells clients the cap, so only abuse sends one, and merging it would cost the tick.
	w.Command("alice", input(1, frames(1, limit+1)...))
	w.Tick()
	if n := len(w.Events[player.BodySync]()); n != 0 {
		t.Fatalf("sent %d body_syncs after an oversized Input, want it dropped before claiming a Body", n)
	}

	// More frames than the cap arrive in one tick, over two Inputs, which a paced client never sends.
	// Only the newest `limit` are kept, so latency stays bounded.
	n := limit + 5
	w.Command("alice", input(1, frames(1, limit)...))
	w.Command("alice", input(1, frames(limit+1, n)...))
	w.Tick()
	s := bodiesFor(t, w, "alice")
	requireAck(t, s, n-limit+1)
	if s.You.InputCap != limit {
		t.Fatalf("YouState.InputCap = %d, want the queue's %d", s.You.InputCap, limit)
	}
	for range limit - 1 {
		w.Tick()
	}
	requireAck(t, bodiesFor(t, w, "alice"), n)
}

func TestSilentClientStops(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", input(1, frame(1, 1, 0)))
	w.Tick()

	// The client goes silent while moving (alt-tab, crash). Its Body coasts for input_timeout_seconds,
	// then stops.
	for range ticks(tuningOf[player.MovementTuning](t).InputTimeoutSeconds) - 1 {
		w.Tick()
	}
	before := bodiesFor(t, w, "alice")
	w.Tick()
	after := bodiesFor(t, w, "alice")
	requireMoved(t, before, after, 1, 0)
	w.Tick()
	requireMoved(t, after, bodiesFor(t, w, "alice"), 0, 0)
}

func TestBadFrameDoesNotMove(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	// Start moving, so a bad frame that was skipped instead would leave the Body coasting.
	w.Command("alice", input(1, frame(1, 1, 0)))
	w.Tick()
	before := bodiesFor(t, w, "alice")

	// A bad frame reads as "no input": the Body stops instead of taking NaN into physics.
	w.Command("alice", input(1, frame(2, math.NaN(), math.Inf(1))))
	w.Tick()
	requireMoved(t, before, bodiesFor(t, w, "alice"), 0, 0)
}

func TestStateOnlyGoesToPlayersWithABody(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Tick()
	n := len(w.Events[player.BodySync]()) + len(w.Events[echo.EchoSync]()) +
		len(w.Events[ability.AbilitySync]()) + len(w.Events[projectile.ProjectileSync]())
	if n != 0 {
		t.Fatalf("sent %d sync events before anyone took a Body", n)
	}

	w.Command("alice", input(1, frame(1, 0, 0)))
	w.Command("bob", input(1, frame(1, 0, 0)))
	w.Tick()
	a, b := bodiesFor(t, w, "alice"), bodiesFor(t, w, "bob")
	if a.You.Body == b.You.Body {
		t.Fatal("alice and bob got the same Body")
	}
	if a.Bodies.Len() != 2 {
		t.Fatalf("alice sees %d Bodies, want 2", a.Bodies.Len())
	}
	if n := sentTo[echo.EchoSync](t, w, "alice").Echoes.Len(); n != 2 {
		t.Fatalf("alice sees %d Echoes, want 2", n)
	}
	sentTo[ability.AbilitySync](t, w, "alice")
	sentTo[projectile.ProjectileSync](t, w, "alice")
}

func TestWallStopsBody(t *testing.T) {
	w, alice, _ := duel(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	r := radiusOf(w, a)
	face := a.X + r + 1 // 1 unit ahead of her
	addWall(w, physics2d.Vec2{X: face + 0.5, Y: a.Y}, physics2d.Vec2{X: 0.5, Y: 5})
	w.Tick() // physics picks the wall up

	// She walks into it, then keeps pushing.
	step := bodiesFor(t, w, "alice").You.MoveSpeed * physics.SecondsPerTick
	for range int(math.Ceil(1/step)) + 1 {
		alice.move(1, 0)
		w.Tick()
	}
	before := bodiesFor(t, w, "alice")
	alice.move(1, 0)
	w.Tick()
	after := bodiesFor(t, w, "alice")
	requireMoved(t, before, after, 0, 0)
	// The mover rests up to LinearSlop (5 mm) inside a face.
	if edge := yourBody(t, after).X + r; math.Abs(edge-face) > 0.01 {
		t.Fatalf("her edge stopped at x = %.4f, want the wall's face at %.4f", edge, face)
	}
}

// Pressing diagonally into a wall slides along it at the part of the input along the wall.
func TestBodySlidesAlongWall(t *testing.T) {
	w, alice, _ := duel(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	addWall(w, physics2d.Vec2{X: a.X + radiusOf(w, a) + 0.5, Y: a.Y}, physics2d.Vec2{X: 0.5, Y: 5})
	w.Tick() // physics picks the wall up; its face touches her right side

	alice.move(1, 1)
	w.Tick() // settles against the wall
	for range 5 {
		before := bodiesFor(t, w, "alice")
		alice.move(1, 1)
		w.Tick()
		requireMoved(t, before, bodiesFor(t, w, "alice"), 0, math.Sqrt2/2)
	}
}

// Bodies don't block each other, for now: two walking into each other pass straight through.
func TestBodiesPassThroughEachOther(t *testing.T) {
	w, alice, bob := duel(t)
	a, b := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	if a.Y != b.Y || a.X >= b.X {
		t.Fatalf("alice at (%.2f, %.2f) doesn't face bob at (%.2f, %.2f) along +x", a.X, a.Y, b.X, b.Y)
	}
	step := bodiesFor(t, w, "alice").You.MoveSpeed * physics.SecondsPerTick
	for range int(math.Ceil((b.X-a.X)/step)) + 1 { // until they've swapped sides
		aliceBefore, bobBefore := bodiesFor(t, w, "alice"), bodiesFor(t, w, "bob")
		alice.move(1, 0)
		bob.move(-1, 0)
		w.Tick()
		requireMoved(t, aliceBefore, bodiesFor(t, w, "alice"), 1, 0)
		requireMoved(t, bobBefore, bodiesFor(t, w, "bob"), -1, 0)
	}
}

// A Body's speed and size are its champion's.
func TestBodyHasItsChampionsNumbers(t *testing.T) {
	w, _, _ := duel(t)
	tuning := tuningOf[player.MovementTuning](t)
	want := tuning.Champions[tuning.DevChampion]
	s := bodiesFor(t, w, "alice")
	if got := (player.ChampionDef{MoveSpeed: s.You.MoveSpeed, Radius: radiusOf(w, yourBody(t, s))}); got != want {
		t.Fatalf("alice's Body has %+v, want her champion's %+v", got, want)
	}
	for b := range s.Bodies.Values() { // so clients draw each Body its size
		if b.Radius != want.Radius {
			t.Fatalf("BodySync sends Body %d with radius %v, want %v", b.EntityID, b.Radius, want.Radius)
		}
	}
}

// Other screens see each Body's champion and the kinds of its effects and its Echo's, never their numbers
// or sources, so a client draws them by ID.
func TestOthersSeeChampionsAndEffectKinds(t *testing.T) {
	w, _, _, fx := fxDuel(t)
	b := yourBody(t, bodiesFor(t, w, "bob"))
	fx.effect(b, effectcomp.EffectIntent{Kind: "stun", Ticks: 5})
	emit(fx, effectcomp.EffectIntent{Target: echoOf(t, w, "alice", b).EntityID, Kind: "slow", Ticks: 3, Value: -2000})
	w.Tick()

	stun := bodiesFor(t, w, "bob").You.Effects.At(0)
	var seen player.BodyState
	for s := range bodiesFor(t, w, "alice").Bodies.Values() {
		if s.EntityID == b.EntityID {
			seen = s
		}
	}
	champion := tuningOf[player.MovementTuning](t).DevChampion
	got := slices.Collect(seen.Effects.Values())
	if seen.Champion != champion || !slices.Equal(got, []status.Shown{{Kind: "stun", End: stun.End}}) {
		t.Fatalf("alice sees bob's Body as %q with %+v, want %q with a stun until %d", seen.Champion, got, champion,
			stun.End)
	}
	got = slices.Collect(echoOf(t, w, "alice", b).Effects.Values())
	if !slices.Equal(got, []status.Shown{{Kind: "slow", End: stun.End - 2}}) {
		t.Fatalf("alice sees bob's Echo with %+v, want a slow until %d", got, stun.End-2)
	}
}
