package test

import (
	"context"
	"echo_image/pkg/level"
	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/echo"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/projectile"
	"echo_image/shards/game/shard"
	"math"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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
	w := cardinal.NewTestWorld(t, withContent(t))

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
	w := cardinal.NewTestWorld(t, withContent(t))
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
	w := cardinal.NewTestWorld(t, withContent(t))
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
	w := cardinal.NewTestWorld(t, withContent(t))
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
	w := cardinal.NewTestWorld(t, withContent(t))
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
	w := cardinal.NewTestWorld(t, withContent(t))
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
	w := cardinal.NewTestWorld(t, withContent(t))
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
	w := cardinal.NewTestWorld(t, withContent(t))
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
	spawn, r := spawnOf(t, 0)
	face := spawn.X + r + 1 // 1 unit ahead of her
	w, alice, _ := duelOn(t, wall(box2d.Vec2{X: face + 0.5, Y: spawn.Y}, box2d.Vec2{X: 0.5, Y: 5}))

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
	spawn, r := spawnOf(t, 0)
	half := box2d.Vec2{X: 0.5, Y: 5}
	// Its face is 1 unit (1/1024 m) from her right side: a map's rules refuse a spawn touching a wall.
	w, alice, _ := duelOn(t, wall(box2d.Vec2{X: spawn.X + r + 1.0/level.Unit + 0.5, Y: spawn.Y}, half))

	alice.move(1, 1)
	w.Tick() // settles against the wall
	for range 5 {
		before := bodiesFor(t, w, "alice")
		alice.move(1, 1)
		w.Tick()
		requireMoved(t, before, bodiesFor(t, w, "alice"), 0, math.Sqrt2/2)
	}

	// 2 mm short of a wall (2 units of 1/1024 m), the tick she meets it still slides the whole way along it.
	w, alice, _ = duelOn(t, wall(box2d.Vec2{X: spawn.X + r + 2.0/level.Unit + 0.5, Y: spawn.Y}, half))
	before := bodiesFor(t, w, "alice")
	alice.move(1, 1)
	w.Tick()
	after := bodiesFor(t, w, "alice")
	want := after.You.MoveSpeed * after.TickSeconds * math.Sqrt2 / 2
	if dy := yourBody(t, after).Y - yourBody(t, before).Y; !(math.Abs(dy-want) <= 1e-4) {
		t.Fatalf("meeting the wall, she slid %.4f along it, want %.4f", dy, want)
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

// What goes wrong with a client's input is counted, on its Body for its HUD and in the shard's metrics
// for ops: a coast, a view that went back, a frame past the cap and an Input over it.
func TestInputHealthIsCounted(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	w := cardinal.NewTestWorld(t, func(world *cardinal.World) {
		shard.RegisterWith(world, content.FS, sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	})
	alice, _ := claim(t, w)
	limit := int(bodiesFor(t, w, "alice").You.InputCap)
	moveFor(w, alice, 1, 0, ticks(0.5)) // past the views before tick 0, which are floored
	before := bodiesFor(t, w, "alice").You.InputHealth
	w.Tick() // no frame: a coast
	alice.move(1, 0)
	w.Tick()
	alice.back += 5
	alice.move(1, 0) // drawn further back than her last frame
	w.Tick()
	alice.back -= 5
	for range limit + 1 {
		alice.move(1, 0)
	}
	frames := make([]playercomp.InputFrame, limit+1)
	for i := range frames {
		frames[i] = frame(uint32(1000+i), 1, 0)
	}
	w.Command("alice", input(alice.session, frames...))
	w.Tick()

	want := playercomp.InputHealth{Coasts: before.Coasts + 1, DroppedFrames: before.DroppedFrames + 1,
		OversizedInputs: before.OversizedInputs + 1, FlooredViews: before.FlooredViews + 1}
	if got := bodiesFor(t, w, "alice").You.InputHealth; got != want {
		t.Fatalf("her HUD gets %+v, want %+v", got, want)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, p := range data.DataPoints {
					got[m.Name] += p.Value
				}
			case metricdata.Histogram[int64]:
				for _, p := range data.DataPoints {
					got[m.Name] += int64(p.Count)
				}
			}
		}
	}
	// The shard's counts are its Bodies' added up.
	a, b := want, bodiesFor(t, w, "bob").You.InputHealth
	for name, n := range map[string]uint32{"echo.input.coasts": a.Coasts + b.Coasts,
		"echo.input.views_floored":  a.FlooredViews + b.FlooredViews,
		"echo.input.frames_dropped": a.DroppedFrames + b.DroppedFrames,
		"echo.input.oversized":      a.OversizedInputs + b.OversizedInputs} {
		if got[name] != int64(n) {
			t.Errorf("%s = %d, want %d", name, got[name], n)
		}
	}
	if got["echo.input.queued"] == 0 {
		t.Error("no queue depths recorded")
	}
}

// Her client keeps a target of frames queued, which YouState sends: a tick at first. Each time her
// queue runs dry while she plays, a coast, it rises a step, up to the buffer. It falls a step for each
// calm period after the last.
func TestQueueTargetRisesWithCoastsAndFallsWhenCalm(t *testing.T) {
	w, alice, _ := duel(t)
	tuning := tuningOf[player.MovementTuning](t)
	step, calm, limit := tuning.QueueStepFrames(), int(tuning.QueueCalmTicks()), tuning.BufferFrames()
	target := func() int { return int(bodiesFor(t, w, "alice").You.InputTarget) }
	alice.move(1, 0)
	w.Tick()
	if got := target(); got != 1 {
		t.Fatalf("target %d frames at first, want 1", got)
	}
	w.Tick() // no frame: her queue runs dry
	w.Tick() // still the same coast
	if got := target(); got != 1+step {
		t.Fatalf("target %d frames after a coast, want %d", got, 1+step)
	}
	for range limit {
		alice.move(1, 0)
		w.Tick()
		w.Tick()
	}
	if got := target(); got != limit {
		t.Fatalf("target %d frames after many coasts, want the buffer, %d", got, limit)
	}
	for i := 1; i <= calm; i++ {
		alice.move(1, 0)
		w.Tick()
		if got, want := target(), limit-step*(i/calm); got != want {
			t.Fatalf("target %d frames %d ticks after the last coast, want %d", got, i, want)
		}
	}
}

// A Body's speed and size, and how its client draws a correction, are its champion's.
func TestBodyHasItsChampionsNumbers(t *testing.T) {
	w, _, _ := duel(t)
	tuning := tuningOf[player.MovementTuning](t)
	want := tuning.Champions[tuning.DevChampion]
	s := bodiesFor(t, w, "alice")
	if got := (player.ChampionDef{MoveSpeed: s.You.MoveSpeed, Radius: radiusOf(w, yourBody(t, s)),
		SnapSeconds: s.You.SnapSeconds, FadeSeconds: s.You.FadeSeconds}); got != want {
		t.Fatalf("alice's Body has %+v, want her champion's %+v", got, want)
	}
	for b := range s.Bodies.Values() { // so clients draw each Body its size
		if r := rowOf(t, w, "alice", b.Serial); r.Radius != want.Radius {
			t.Fatalf("TargetSync sends Body %d with radius %v, want %v", b.EntityID, r.Radius, want.Radius)
		}
	}
}

// Other screens see each Body's champion and the kinds of its effects and its Echo's, never their numbers
// or sources, so a client draws them by ID.
func TestOthersSeeChampionsAndEffectKinds(t *testing.T) {
	w, _, _, fx := fxDuel(t)
	b := yourBody(t, bodiesFor(t, w, "bob"))
	fx.effect(b, effectcomp.EffectIntent{Kind: "stun", Ticks: 5})
	send(fx, effectcomp.EffectIntent{Target: echoOf(t, w, "alice", b).EntityID, Kind: "slow", Ticks: 3, Value: -2000})
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

// Each team's Bodies start on its spawns in the map, one each, in file order.
func TestBodiesStartOnTheirTeamsSpawns(t *testing.T) {
	w := cardinal.NewTestWorld(t, withFiles(twoVsTwo(t)))
	claim(t, w)
	join(t, w, "carol")
	join(t, w, "dave")
	for name, x := range map[string]float64{"alice": -5, "bob": 5, "carol": -3, "dave": 3} {
		if b := yourBody(t, bodiesFor(t, w, name)); b.X != x || b.Y != 0 {
			t.Errorf("%s starts at (%v, %v), want (%v, 0)", name, b.X, b.Y, x)
		}
	}
}

// body_sync names the live map, so a client draws and predicts with its file: here a second map, which
// content makes live at boot.
func TestBodySyncNamesTheLiveMap(t *testing.T) {
	open, err := content.FS.ReadFile("arena/open.level.json")
	if err != nil {
		t.Fatal(err)
	}
	w := cardinal.NewTestWorld(t, withFiles(content.Overlay{
		"arena/tuning.json":     []byte(`{"maps": ["open", "copy"], "default_map": "copy"}`),
		"arena/copy.level.json": open,
	}))
	claim(t, w)
	if got := bodiesFor(t, w, "alice").MapID; got != "copy" {
		t.Fatalf("body_sync names map %q, want copy", got)
	}
}
