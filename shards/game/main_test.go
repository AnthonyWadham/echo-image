package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/vmihailenco/msgpack/v5"

	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/projectile"
	"echo_image/shards/game/internal/team"
)

func input(session uint32, frames ...playercomp.InputFrame) player.Input {
	return player.Input{Session: session, Frames: cardinal.SliceOf(frames...)}
}

func frame(seq uint32, x, y float64) playercomp.InputFrame {
	return playercomp.InputFrame{Seq: seq, MoveX: x, MoveY: y}
}

// tuningOf reads the shipped tuning for T, so tests follow the real numbers.
func tuningOf[T interface{ JSONFile() string }](t *testing.T) T {
	t.Helper()
	var v T
	raw, err := content.FS.ReadFile(v.JSONFile())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// sentTo returns the T the last tick sent to player, failing if any T was broadcast or player didn't
// get exactly one.
func sentTo[T cardinal.Event](t *testing.T, w *cardinal.TestWorld, playerID string) T {
	t.Helper()
	var found []T
	for _, sent := range w.Events[T]() {
		if sent.Recipient == "" {
			t.Fatalf("%s was broadcast; it must be sent per player", sent.Payload.Name())
		}
		if sent.Recipient == playerID {
			found = append(found, sent.Payload)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want 1 %T for %s, got %d", *new(T), playerID, len(found))
	}
	return found[0]
}

func bodiesFor(t *testing.T, w *cardinal.TestWorld, playerID string) player.BodySync {
	t.Helper()
	return sentTo[player.BodySync](t, w, playerID)
}

func requireAck(t *testing.T, s player.BodySync, want uint32) {
	t.Helper()
	if s.You.InputAck != want {
		t.Fatalf("InputAck = %d, want %d", s.You.InputAck, want)
	}
}

// yourBody returns the receiving player's own Body.
func yourBody(t *testing.T, s player.BodySync) player.BodyState {
	t.Helper()
	for _, b := range s.Bodies.All() {
		if b.EntityID == s.You.Body {
			return b
		}
	}
	t.Fatalf("your Body %d is not in the state", s.You.Body)
	return player.BodyState{}
}

// requireMoved fails unless the Body moved exactly one tick of full-speed input along (dx, dy).
func requireMoved(t *testing.T, before, after player.BodySync, dx, dy float64) {
	t.Helper()
	b0, b1 := yourBody(t, before), yourBody(t, after)
	step := after.You.MoveSpeed * after.TickSeconds
	const eps = 1e-4
	// Written as !(within) so a NaN position fails too.
	if !(math.Abs(b1.X-b0.X-dx*step) <= eps && math.Abs(b1.Y-b0.Y-dy*step) <= eps) {
		t.Fatalf("moved (%.4f, %.4f), want (%.4f, %.4f)", b1.X-b0.X, b1.Y-b0.Y, dx*step, dy*step)
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
	w.Command("alice", input(1, frame(3, 0, 1)))
	w.Command("alice", input(1, frame(2, 1, 0)))
	w.Tick()
	a := bodiesFor(t, w, "alice")
	requireAck(t, a, 2)
	requireMoved(t, start, a, 1, 0)

	w.Tick()
	b := bodiesFor(t, w, "alice")
	requireAck(t, b, 3)
	requireMoved(t, a, b, 0, 1)
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

	// After a stall, 10 frames arrive at once. Only the newest `limit` are kept, so latency stays bounded.
	var frames []playercomp.InputFrame
	for seq := uint32(1); seq <= 10; seq++ {
		frames = append(frames, frame(seq, 1, 0))
	}
	w.Command("alice", input(1, frames...))
	w.Tick()
	requireAck(t, bodiesFor(t, w, "alice"), 10-limit+1)
	for range limit - 1 {
		w.Tick()
	}
	requireAck(t, bodiesFor(t, w, "alice"), 10)
}

func TestSilentClientStops(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", input(1, frame(1, 1, 0)))
	w.Tick()

	// The client goes silent while moving (alt-tab, crash). After the timeout its Body stops.
	for range tuningOf[player.MovementTuning](t).TimeoutTicks() + 1 {
		w.Tick()
	}
	before := bodiesFor(t, w, "alice")
	w.Tick()
	requireMoved(t, before, bodiesFor(t, w, "alice"), 0, 0)
}

func TestBadFrameDoesNotMove(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	// Start moving: Box2D won't wake a sleeping Body for a NaN velocity, which would hide the bug.
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

// The player sync must not depend on movement internals: a Body without them is still shown.
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

	s := bodiesFor(t, w, "carol")
	if s.You.Body != uint32(e.ID()) || s.You.InputAck != 0 || s.You.MoveSpeed != 0 {
		t.Fatalf("You = %+v, want Body %d with zero ack and speed", s.You, e.ID())
	}
}

// --- Abilities and projectiles ---

// client sends one input frame per call, numbering them the way the Unity client does.
type client struct {
	w   *cardinal.TestWorld
	id  string
	seq uint32
}

func (c *client) send(f playercomp.InputFrame) {
	c.seq++
	f.Seq = c.seq
	c.w.Command(c.id, input(1, f))
}

func (c *client) move(x, y float64) { c.send(playercomp.InputFrame{MoveX: x, MoveY: y}) }

// fire holds button 0, which casts kit slot 0.
func (c *client) fire(aimX, aimY float64) {
	c.send(playercomp.InputFrame{AimX: aimX, AimY: aimY, Buttons: 1})
}

// primary returns the tuning of the ability in kit slot 0 and of the shot it launches.
func primary(t *testing.T) (ability.AbilityDef, projectile.Launcher) {
	t.Helper()
	abilities := tuningOf[ability.AbilityTuning](t)
	id := abilities.DefaultKit[0]
	launcher, ok := tuningOf[projectile.ProjectileTuning](t).Launchers[id]
	if !ok {
		t.Fatalf("kit slot 0 holds %q, which launches no shot", id)
	}
	return abilities.Abilities[id], launcher
}

// duel has alice and bob take their Bodies (on opposite teams), standing still.
func duel(t *testing.T) (w *cardinal.TestWorld, alice, bob *client) {
	t.Helper()
	w = cardinal.NewTestWorld(t, register)
	alice, bob = &client{w: w, id: "alice"}, &client{w: w, id: "bob"}
	alice.move(0, 0)
	bob.move(0, 0)
	w.Tick()
	if yourBody(t, bodiesFor(t, w, "alice")).Team == yourBody(t, bodiesFor(t, w, "bob")).Team {
		t.Fatal("alice and bob are on the same team")
	}
	return w, alice, bob
}

// echoOf returns the Echo of the player who owns the Body in s, as viewer sees it.
func echoOf(t *testing.T, w *cardinal.TestWorld, viewer string, owner player.BodyState) echo.EchoState {
	t.Helper()
	for _, e := range sentTo[echo.EchoSync](t, w, viewer).Echoes.All() {
		if e.PlayerID == owner.PlayerID {
			return e
		}
	}
	t.Fatalf("no Echo for player %d", owner.PlayerID)
	return echo.EchoState{}
}

// fireAtEcho has alice fire at bob's Echo, checks the shot left, and returns how many more ticks it
// needs to reach that Echo. That's less than its range, so a shot gone by then has hit something.
func fireAtEcho(t *testing.T, w *cardinal.TestWorld, alice *client) int {
	t.Helper()
	from := yourBody(t, bodiesFor(t, w, "alice"))
	to := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	alice.fire(to.X-from.X, to.Y-from.Y)
	w.Tick()
	if n := len(shots(t, w, "alice")); n != 1 {
		t.Fatalf("%d shots after firing, want 1", n)
	}
	_, shot := primary(t)
	step := shot.Speed * physics.SecondsPerTick
	dist := math.Hypot(to.X-from.X, to.Y-from.Y)
	if dist >= shot.Range-step {
		t.Fatalf("bob's Echo is %.1f away, beyond the shot's range", dist)
	}
	return int(math.Ceil(dist / step))
}

// moveFor sends c's direction for n ticks, then stops it.
func moveFor(w *cardinal.TestWorld, c *client, x, y float64, n int) {
	for range n {
		c.move(x, y)
		w.Tick()
	}
	c.move(0, 0)
	w.Tick()
}

func shots(t *testing.T, w *cardinal.TestWorld, viewer string) []projectile.ProjectileState {
	t.Helper()
	return slices.Collect(sentTo[projectile.ProjectileSync](t, w, viewer).Projectiles.Values())
}

// readyTick returns when body's kit slot 0 is ready, as viewer sees it.
func readyTick(t *testing.T, w *cardinal.TestWorld, viewer string, body uint32) (uint64, bool) {
	t.Helper()
	for _, s := range sentTo[ability.AbilitySync](t, w, viewer).Abilities.All() {
		if s.Body == body && s.Slot == 0 {
			return s.ReadyTick, true
		}
	}
	return 0, false
}

func TestFireStartsCooldownAndEveryoneSeesTheShot(t *testing.T) {
	w, alice, _ := duel(t)
	def, _ := primary(t)
	cooldown := def.CooldownTicks()

	fired := w.TickHeight()
	alice.fire(0, 1)
	w.Tick()

	ready, ok := readyTick(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice")).EntityID)
	if !ok || ready != fired+cooldown {
		t.Fatalf("alice's ReadyTick = %d (found %v), want %d", ready, ok, fired+cooldown)
	}
	if len(shots(t, w, "alice")) != 1 || len(shots(t, w, "bob")) != 1 {
		t.Fatalf("alice sees %d shots and bob %d, want 1 each", len(shots(t, w, "alice")), len(shots(t, w, "bob")))
	}
}

func TestHoldingFireShootsOncePerCooldown(t *testing.T) {
	w, alice, _ := duel(t)
	def, _ := primary(t)
	cooldown := def.CooldownTicks()
	body := yourBody(t, bodiesFor(t, w, "alice")).EntityID

	// Fire held every tick: the first shot sets ReadyTick, and nothing fires again until it.
	fired := w.TickHeight()
	for i := range cooldown + 1 {
		alice.fire(0, 1)
		w.Tick()
		want := fired + cooldown
		if i == cooldown {
			want += cooldown // fired again the tick the cooldown ended
		}
		if got, _ := readyTick(t, w, "alice", body); got != want {
			t.Fatalf("after %d ticks of fire, ReadyTick = %d, want %d", i+1, got, want)
		}
	}
}

func TestCooldownGoesOnlyToAllies(t *testing.T) {
	w, alice, _ := duel(t)
	aliceBody := yourBody(t, bodiesFor(t, w, "alice"))

	// carol joins alice's team.
	type allyRow struct {
		Player    playercomp.Player
		Team      team.Team
		Transform physics2d.Transform2D
	}
	carol := w.Create[allyRow]()
	carol.Set(playercomp.Player{ID: 9, Persona: "carol", Nickname: "Carol"})
	carol.Set(team.Team{ID: aliceBody.Team})

	alice.fire(0, 1)
	w.Tick()
	for viewer, want := range map[string]bool{"alice": true, "carol": true, "bob": false} {
		if _, ok := readyTick(t, w, viewer, aliceBody.EntityID); ok != want {
			t.Errorf("%s gets alice's cooldown: %v, want %v", viewer, ok, want)
		}
	}
}

func TestBadAimDoesNotFire(t *testing.T) {
	w, alice, _ := duel(t)
	alice.fire(math.NaN(), math.Inf(1))
	w.Tick()
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("%d shots fired with a bad aim, want 0", n)
	}
	if ready, _ := readyTick(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice")).EntityID); ready != 0 {
		t.Fatalf("ReadyTick = %d after a bad aim, want 0 (no cooldown spent)", ready)
	}
}

// Button N casts kit slot N, so another button never casts slot 0.
func TestButtonCastsOnlyItsSlot(t *testing.T) {
	w, alice, _ := duel(t)
	alice.send(playercomp.InputFrame{AimX: 0, AimY: 1, Buttons: 1 << 1})
	w.Tick()
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("%d shots from button 1, want 0", n)
	}
	if ready, _ := readyTick(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice")).EntityID); ready != 0 {
		t.Fatalf("slot 0 ReadyTick = %d after button 1, want 0", ready)
	}
}

// flyFor ticks n times with no input.
func flyFor(w *cardinal.TestWorld, n int) {
	for range n {
		w.Tick()
	}
}

func TestShotDamagesEnemyEcho(t *testing.T) {
	w, alice, bob := duel(t)
	// bob steps off his Echo, which stays at his spawn while its trail warms up.
	moveFor(w, bob, 0, 1, 10)

	flyFor(w, fireAtEcho(t, w, alice))

	bobBody := yourBody(t, bodiesFor(t, w, "bob"))
	got := echoOf(t, w, "alice", bobBody)
	if _, shot := primary(t); got.HP != got.MaxHP-shot.Damage {
		t.Fatalf("bob's Echo HP = %v, want %v", got.HP, got.MaxHP-shot.Damage)
	}
	if mine := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice"))); mine.HP != mine.MaxHP {
		t.Fatalf("alice's own Echo took damage: HP %v", mine.HP)
	}
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("%d shots left after the hit, want 0", n)
	}
}

func TestEnemyBodyBlocksShot(t *testing.T) {
	w, alice, bob := duel(t)
	// bob steps toward alice, between her and his Echo.
	moveFor(w, bob, -1, 0, 10)

	flyFor(w, fireAtEcho(t, w, alice))

	got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	if got.HP != got.MaxHP {
		t.Fatalf("bob's Echo HP = %v behind his Body, want %v", got.HP, got.MaxHP)
	}
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("%d shots left after the block, want 0", n)
	}
}

// A Body standing on its own Echo touches the shot at the same moment; the Body wins the tie.
func TestBodyOnItsEchoShieldsIt(t *testing.T) {
	w, alice, _ := duel(t)
	flyFor(w, fireAtEcho(t, w, alice))

	if got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))); got.HP != got.MaxHP {
		t.Fatalf("bob's Echo HP = %v under his Body, want %v", got.HP, got.MaxHP)
	}
}

func TestShotPassesThroughAllies(t *testing.T) {
	w, alice, _ := duel(t)
	from := yourBody(t, bodiesFor(t, w, "alice"))

	// carol, alice's teammate, stands 2 units in front of her.
	type allyBodyRow struct {
		Team      team.Team
		Transform physics2d.Transform2D
		Velocity  physics2d.Velocity2D
		Body      physics2d.PhysicsBody2D
	}
	carol := w.Create[allyBodyRow]()
	carol.Set(team.Team{ID: from.Team})
	carol.Set(physics2d.Transform2D{Position: physics2d.Vec2{X: from.X + 2, Y: from.Y}})
	carol.Set(physics.NewBody2D(physics2d.BodyTypeKinematic, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       0.5,
		CategoryBits: physics.CategoryBody,
		MaskBits:     physics.CategoryBody | physics.CategoryEnvironment,
	}))

	// The shot starts inside alice's own Body and Echo, then crosses carol.
	alice.fire(1, 0)
	w.Tick()
	flyFor(w, 4)

	s := shots(t, w, "alice")
	if len(s) != 1 || s[0].X <= from.X+2.5 {
		t.Fatalf("shots = %+v, want one past carol (x > %.2f)", s, from.X+2.5)
	}
}

func TestShotExpiresAtRange(t *testing.T) {
	w, alice, _ := duel(t)
	_, shot := primary(t)
	steps := int(math.Ceil(shot.Range / (shot.Speed * physics.SecondsPerTick)))

	alice.fire(0, 1) // nothing above her
	w.Tick()         // first step
	flyFor(w, steps-2)
	if n := len(shots(t, w, "alice")); n != 1 {
		t.Fatalf("shot gone after %d of %d steps", steps-1, steps)
	}
	w.Tick()
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("shot still flying after %d steps, past its range", steps)
	}
}

func TestWallStopsShot(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, 10) // off his Echo, so only the wall can stop the shot

	// A wall halfway between them.
	a, b := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	type wallRow struct {
		Transform physics2d.Transform2D
		Velocity  physics2d.Velocity2D
		Body      physics2d.PhysicsBody2D
	}
	wall := w.Create[wallRow]()
	wall.Set(physics2d.Transform2D{Position: physics2d.Vec2{X: (a.X + b.X) / 2, Y: 0}})
	wall.Set(physics.NewBody2D(physics2d.BodyTypeStatic, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeBox,
		HalfExtents:  physics2d.Vec2{X: 0.5, Y: 5},
		CategoryBits: physics.CategoryEnvironment,
		MaskBits:     physics.CategoryBody | physics.CategoryProjectile,
	}))
	w.Tick() // physics picks the wall up

	flyFor(w, fireAtEcho(t, w, alice))

	if got := echoOf(t, w, "alice", b); got.HP != got.MaxHP {
		t.Fatalf("bob's Echo HP = %v behind a wall, want %v", got.HP, got.MaxHP)
	}
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("%d shots left after the wall, want 0", n)
	}
}

// A frame acts once: a client that fired and went silent doesn't keep firing on its last frame.
func TestFireNeedsANewFrame(t *testing.T) {
	w, alice, _ := duel(t)
	def, _ := primary(t)
	cooldown := def.CooldownTicks()
	body := yourBody(t, bodiesFor(t, w, "alice")).EntityID

	fired := w.TickHeight()
	alice.fire(0, 1)
	w.Tick()
	flyFor(w, int(cooldown)+1)
	if got, _ := readyTick(t, w, "alice", body); got != fired+cooldown {
		t.Fatalf("ReadyTick = %d after going silent, want %d (fired once)", got, fired+cooldown)
	}
}

// A shot that starts inside an enemy Echo hits it at once, whichever way it's aimed.
func TestPointBlankShotHitsEcho(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, 10) // off his Echo
	a := yourBody(t, bodiesFor(t, w, "alice"))
	target := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))

	// alice walks onto bob's Echo.
	speed := bodiesFor(t, w, "alice").You.MoveSpeed * physics.SecondsPerTick
	moveFor(w, alice, 1, 0, int(math.Round((target.X-a.X)/speed)))
	if a = yourBody(t, bodiesFor(t, w, "alice")); math.Hypot(a.X-target.X, a.Y-target.Y) > 0.1 {
		t.Fatalf("alice stopped at (%.2f, %.2f), not on bob's Echo at (%.2f, %.2f)", a.X, a.Y, target.X, target.Y)
	}

	alice.fire(0, 1) // away from the Echo's center
	w.Tick()
	_, shot := primary(t)
	if got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))); got.HP != got.MaxHP-shot.Damage {
		t.Fatalf("bob's Echo HP = %v after a point-blank shot, want %v", got.HP, got.MaxHP-shot.Damage)
	}
}

// Features key their numbers by ability ID. A launcher whose ability is missing never fires, and one
// whose ability isn't aimed can be cast with no direction to fly in.
func TestLaunchersAreAimedAbilities(t *testing.T) {
	abilities := tuningOf[ability.AbilityTuning](t).Abilities
	for id := range tuningOf[projectile.ProjectileTuning](t).Launchers {
		if def, ok := abilities[id]; !ok || !def.Aimed {
			t.Errorf("launcher %q: ability found %v, aimed %v; want an aimed ability", id, ok, def.Aimed)
		}
	}
}

// --- Wire format ---

var update = flag.Bool("update", false, "rewrite testdata/wire from the TestWireFixtures samples")

// TestWireFixtures pins each payload's bytes in testdata/wire, which the client checks too
// (client/src/game/wire_fixtures.test.ts). A renamed field fails here instead of silently zeroing on
// the other side.
func TestWireFixtures(t *testing.T) {
	in := player.Input{Session: 305419896, Frames: cardinal.SliceOf(
		playercomp.InputFrame{Seq: 1, MoveX: 1},
		playercomp.InputFrame{
			Seq: 2, MoveX: 0.7071067811865476, MoveY: -0.7071067811865476, AimX: 3.5, AimY: -1.25, Buttons: 1,
		},
	)}
	samples := []struct {
		name    string
		payload any
	}{
		{"body_sync", player.BodySync{
			Tick:        9007199254740993, // 2^53+1, past float64 precision
			TickSeconds: 0.05,
			You:         player.YouState{Body: 1, InputSession: 4294967295, InputAck: 42, MoveSpeed: 5},
			Bodies: cardinal.SliceOf(
				player.BodyState{EntityID: 1, PlayerID: 1, Nickname: "Player One", Team: 0, X: 1.5, Y: -2},
				player.BodyState{EntityID: 2, PlayerID: 2, Nickname: "Échø ✦", Team: 1, X: -0.25, Y: 3.75},
			),
		}},
		{"echo_sync", echo.EchoSync{Tick: 7, Echoes: cardinal.SliceOf(
			echo.EchoState{EntityID: 3, PlayerID: 1, Team: 0, X: -4.5, Y: 0.5, HP: 62.5, MaxHP: 100},
		)}},
		{"ability_sync", ability.AbilitySync{Tick: 7, Abilities: cardinal.SliceOf(
			ability.AbilityState{Body: 1, Slot: 0, Ability: "bolt", ReadyTick: 18446744073709551615},
		)}},
		{"projectile_sync", projectile.ProjectileSync{Tick: 8, Projectiles: cardinal.SliceOf(
			projectile.ProjectileState{EntityID: 9, Team: 1, X: 0.125, Y: -7.5, Radius: 0.15},
		)}},
		{"projectile_sync_empty", projectile.ProjectileSync{Tick: 9}},
		{"input", in},
	}
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			got, err := msgpack.Marshal(s.payload) // as Cardinal's schema.Serialize does
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "wire", s.name+".msgpack")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%T no longer encodes as %s. If intended, run "+
					"`go test ./shards/game -run TestWireFixtures -update` and update its type in client/src/game",
					s.payload, path)
			}
		})
	}

	// The client's own encoding of the input sample, written by client/src/game/wire_fixtures.test.ts.
	t.Run("input_client", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("testdata", "wire", "input_client.msgpack"))
		if err != nil {
			t.Fatal(err)
		}
		var got player.Input
		if err := msgpack.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		gotFrames, wantFrames := slices.Collect(got.Frames.Values()), slices.Collect(in.Frames.Values())
		if got.Session != in.Session || !slices.Equal(gotFrames, wantFrames) {
			t.Fatalf("the client's Input decodes as Session %d, Frames %+v; want %d, %+v. Update "+
				"client/src/game/player/input_command.ts, then run "+
				"`UPDATE_FIXTURES=1 bun x vitest run src/game/wire_fixtures.test.ts` in client",
				got.Session, gotFrames, in.Session, wantFrames)
		}
	})
}
