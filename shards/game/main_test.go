package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/vmihailenco/msgpack/v5"

	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/ability/cast"
	abilitycomp "echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/ability/hooks"
	"echo_image/shards/game/internal/area"
	areacomp "echo_image/shards/game/internal/area/component"
	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/effect"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/predict"
	"echo_image/shards/game/internal/projectile"
	projectilecomp "echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
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

// withContent is register with each patch deep-merged into the shipped content file it names, so a
// test can add a shot type or a champion. Objects merge, and anything else replaces.
func withContent(t *testing.T, patches map[string]string) func(*cardinal.World) {
	t.Helper()
	files := overlay{}
	for file, patch := range patches {
		var base, add map[string]any
		raw, err := content.FS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &base); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(patch), &add); err != nil {
			t.Fatalf("patch for %s: %v", file, err)
		}
		merge(base, add)
		if files[file], err = json.Marshal(base); err != nil {
			t.Fatal(err)
		}
	}
	return func(world *cardinal.World) { registerWith(world, files) }
}

func merge(dst, src map[string]any) {
	for k, v := range src {
		if sub, ok := v.(map[string]any); ok {
			if d, ok := dst[k].(map[string]any); ok {
				merge(d, sub)
				continue
			}
		}
		dst[k] = v
	}
}

// overlay serves the shipped content, with some files replaced.
type overlay map[string][]byte

func (o overlay) Fetch(ctx context.Context, file, hash string) ([]byte, string, error) {
	if raw, ok := o[file]; ok {
		sum := sha256.Sum256(raw)
		return raw, hex.EncodeToString(sum[:]), nil
	}
	return data.EmbedSource{FS: content.FS}.Fetch(ctx, file, hash)
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

// cast holds the button for kit slot, aiming (aimX, aimY).
func (c *client) cast(slot int, aimX, aimY float64) {
	c.send(playercomp.InputFrame{AimX: aimX, AimY: aimY, Buttons: 1 << slot})
}

// fire casts kit slot 0.
func (c *client) fire(aimX, aimY float64) { c.cast(0, aimX, aimY) }

// kit returns the ability IDs of the champion dev Bodies play, slot 0 first.
func kit(t *testing.T) []string {
	t.Helper()
	return tuningOf[ability.AbilityTuning](t).Kits[tuningOf[player.MovementTuning](t).DevChampion]
}

// primary returns the tuning of the ability in kit slot 0 and of the shot it launches.
func primary(t *testing.T) (cast.Def, flight.Shot) {
	t.Helper()
	id := kit(t)[0]
	def := tuningOf[ability.AbilityTuning](t).Abilities[id]
	hook, err := hooks.Bind(def.Hook, def.Params, physics.ServerTickRateHz)
	if err != nil || len(hook.Refs.Shots) != 1 {
		t.Fatalf("kit slot 0 holds %q, which launches no one shot (%v)", id, err)
	}
	return def, tuningOf[projectile.ProjectileTuning](t).Shots[hook.Refs.Shots[0]]
}

// duel has alice and bob take their Bodies (on opposite teams), standing still.
func duel(t *testing.T) (w *cardinal.TestWorld, alice, bob *client) {
	t.Helper()
	w = cardinal.NewTestWorld(t, register)
	alice, bob = claim(t, w)
	return w, alice, bob
}

// claim has alice and bob take their Bodies in w (on opposite teams), standing still.
func claim(t *testing.T, w *cardinal.TestWorld) (alice, bob *client) {
	t.Helper()
	alice, bob = &client{w: w, id: "alice"}, &client{w: w, id: "bob"}
	alice.move(0, 0)
	bob.move(0, 0)
	w.Tick()
	if yourBody(t, bodiesFor(t, w, "alice")).Team == yourBody(t, bodiesFor(t, w, "bob")).Team {
		t.Fatal("alice and bob are on the same team")
	}
	return alice, bob
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

	body := yourBody(t, bodiesFor(t, w, "alice")).EntityID
	ready, ok := readyTick(t, w, "alice", body)
	if !ok || ready != fired+cooldown {
		t.Fatalf("alice's ReadyTick = %d (found %v), want %d", ready, ok, fired+cooldown)
	}
	if len(shots(t, w, "alice")) != 1 || len(shots(t, w, "bob")) != 1 {
		t.Fatalf("alice sees %d shots and bob %d, want 1 each", len(shots(t, w, "alice")), len(shots(t, w, "bob")))
	}
	// Her client draws it from her predictor instead, by Caster.
	if c := shots(t, w, "bob")[0].Caster; c != body {
		t.Fatalf("the shot's Caster = %d, want alice's Body %d", c, body)
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

// A shot's damage carries the cast that fired it, and credits the Body that cast it.
func TestShotDamageCarriesItsCast(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, 10) // off his Echo
	ticks := fireAtEcho(t, w, alice)
	casts := w.Emitted[abilitycomp.AbilityCast]()
	if len(casts) != 1 {
		t.Fatalf("%d casts, want 1", len(casts))
	}
	var hits []combatcomp.DamageIntent
	for range ticks {
		w.Tick()
		if hits = w.Emitted[combatcomp.DamageIntent](); len(hits) > 0 {
			break
		}
	}
	target := cardinal.EntityID(echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))).EntityID)
	if len(hits) != 1 || hits[0].Target != target || hits[0].Credit != casts[0].Caster || hits[0].Cause != casts[0].Cause {
		t.Fatalf("damage %+v, want one on bob's Echo %d, credited to and caused by %+v", hits, target, casts[0].Cause)
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
		Owner     team.Owner
		Transform physics2d.Transform2D
		Velocity  physics2d.Velocity2D
		Body      physics2d.PhysicsBody2D
		Effects   effectcomp.Effects
	}
	carol := w.Create[allyBodyRow]()
	carol.Set(team.Team{ID: from.Team})
	carol.Set(team.Owner{Body: carol.ID()})
	carol.Set(physics2d.Transform2D{Position: physics2d.Vec2{X: from.X + 2, Y: from.Y}})
	carol.Set(physics.NewBody2D(physics2d.BodyTypeKinematic, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeCircle,
		Radius:       0.5,
		CategoryBits: uint64(interact.Body),
		MaskBits:     uint64(interact.Body | interact.Wall | interact.Shot),
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

// A shot ends at its range in every direction, though a rounded speed can leave its last step short.
func TestShotExpiresAtRange(t *testing.T) {
	fx := &emitter{}
	w := cardinal.NewTestWorld(t, withEmitter(register, fx))
	claim(t, w)
	def, shot := primary(t)
	hook, _ := hooks.Bind(def.Hook, def.Params, physics.ServerTickRateHz) // primary checked it
	steps := int(math.Ceil(shot.Range / (shot.Speed * physics.SecondsPerTick)))

	const n = 16 // 6 of these got stuck at a remainder above 0
	for k := range n {
		a := 2 * math.Pi * float64(k) / n
		emit(fx, projectilecomp.LaunchIntent{Shot: hook.Refs.Shots[0], Cause: abilitycomp.Cause{Tick: uint64(k)},
			From: physics2d.Vec2{X: 100, Y: 100}, Dir: physics2d.Vec2{X: math.Cos(a), Y: math.Sin(a)}}) // far from everyone
	}
	w.Tick() // first step
	flyFor(w, steps-2)
	if got := len(shots(t, w, "alice")); got != n {
		t.Fatalf("%d of %d shots left after %d of %d steps", got, n, steps-1, steps)
	}
	w.Tick()
	if got := len(shots(t, w, "alice")); got != 0 {
		t.Fatalf("%d shots still flying after %d steps, past their range", got, steps)
	}
}

func TestWallStopsShot(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, 10) // off his Echo, so only the wall can stop the shot

	// A wall halfway between them.
	a, b := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	addWall(w, physics2d.Vec2{X: (a.X + b.X) / 2, Y: 0}, physics2d.Vec2{X: 0.5, Y: 5})
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

// A shot meets an Echo where its replay puts it this tick, not a step behind: the replay runs before
// the physics step, so Box2D holds the new pose when shots query it.
func TestShotMeetsEchoWhereItIsNow(t *testing.T) {
	w, _, bob, fx := fxDuel(t)
	body := yourBody(t, bodiesFor(t, w, "bob"))
	// bob walks until his Echo walks after him, the same step each tick.
	var xs []float64
	steady := func() bool {
		n := len(xs)
		return n >= 3 && xs[n-1] > xs[n-2] && xs[n-1]-xs[n-2] == xs[n-2]-xs[n-3]
	}
	for !steady() {
		if len(xs) > 200 {
			t.Fatal("bob's Echo never walked")
		}
		bob.move(1, 0)
		w.Tick()
		xs = append(xs, echoOf(t, w, "alice", body).X)
	}
	now := echoOf(t, w, "alice", body)
	step := xs[len(xs)-1] - xs[len(xs)-2]

	// alice's shot flies up past the Echo's leading edge next tick: it touches the Echo there, and
	// would miss it a step behind.
	def, shot := primary(t)
	hook, _ := hooks.Bind(def.Hook, def.Params, physics.ServerTickRateHz) // primary checked it
	a := yourBody(t, bodiesFor(t, w, "alice"))
	x := now.X + step + tuningOf[echo.EchoTuning](t).HitRadius + shot.Radius - step/2
	emit(fx, projectilecomp.LaunchIntent{Shot: hook.Refs.Shots[0], Owner: cardinal.EntityID(a.EntityID), Team: a.Team,
		From: physics2d.Vec2{X: x, Y: now.Y - 0.3}, Dir: physics2d.Vec2{Y: 1}})
	bob.move(1, 0)
	w.Tick()
	if got := echoOf(t, w, "alice", body); got.HP != got.MaxHP-shot.Damage {
		t.Fatalf("bob's Echo HP = %v, want %v: the shot missed where it is now", got.HP, got.MaxHP-shot.Damage)
	}
}

// shootEcho has alice fire kit slot 0 at bob's Echo and ticks until the shot is gone, and returns his
// Echo's HP then.
func shootEcho(t *testing.T, w *cardinal.TestWorld, alice *client) float64 {
	t.Helper()
	from, bob := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	to := echoOf(t, w, "alice", bob)
	alice.fire(to.X-from.X, to.Y-from.Y)
	for range 100 {
		w.Tick()
		if len(shots(t, w, "alice")) == 0 {
			return echoOf(t, w, "alice", bob).HP
		}
	}
	t.Fatal("the shot is still flying after 100 ticks")
	return 0
}

// A shot type's own rules come before the defaults: a shot that passes Bodies hits the Echo behind an
// enemy Body (test content).
func TestShotTypeRulesComeFirst(t *testing.T) {
	w := cardinal.NewTestWorld(t, withContent(t, map[string]string{
		"ability/tuning.json": `{"kits": {"starter": ["lance"]},
			"abilities": {"lance": {"aimed": true, "hook": "launch", "params": {"shot": "lance"}}}}`,
		"projectile/tuning.json": `{"shots": {"lance": {"speed": 15, "range": 20, "radius": 0.15, "damage": 10,
			"meets": [{"kinds": ["body"], "do": "pass"}]}}}`,
	}))
	alice, bob := claim(t, w)
	moveFor(w, bob, -1, 0, 10) // between alice and his Echo
	full := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))).MaxHP
	if hp := shootEcho(t, w, alice); hp != full-10 {
		t.Fatalf("bob's Echo HP = %v behind his Body, want %v: the shot passes Bodies", hp, full-10)
	}
}

// Shot rules see the tags a target's effects give it: a shot that passes only stunned Bodies (test
// content).
func TestShotRulesSeeTags(t *testing.T) {
	fx := &emitter{}
	w := cardinal.NewTestWorld(t, withEmitter(withContent(t, map[string]string{
		"ability/tuning.json": `{"kits": {"starter": ["needle"]},
			"abilities": {"needle": {"aimed": true, "hook": "launch", "params": {"shot": "needle"}}}}`,
		"projectile/tuning.json": `{"shots": {"needle": {"speed": 15, "range": 20, "radius": 0.15, "damage": 10,
			"meets": [{"kinds": ["body"], "require": ["stunned"], "do": "pass"}]}}}`,
	}), fx))
	alice, bob := claim(t, w)
	moveFor(w, bob, -1, 0, 10) // between alice and his Echo
	full := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))).MaxHP
	if hp := shootEcho(t, w, alice); hp != full {
		t.Fatalf("bob's Echo HP = %v behind his Body, want %v: it blocks while not stunned", hp, full)
	}
	fx.effect(yourBody(t, bodiesFor(t, w, "bob")), effectcomp.EffectIntent{Kind: "stun", Ticks: 100})
	w.Tick() // the stun applies from the next tick
	if hp := shootEcho(t, w, alice); hp != full-10 {
		t.Fatalf("bob's Echo HP = %v behind his stunned Body, want %v", hp, full-10)
	}
}

// Shots launched in one tick get the same entity IDs whatever order their intents arrive in, since that
// order can change after a restore.
func TestLaunchOrderIsFixed(t *testing.T) {
	a := projectilecomp.LaunchIntent{Shot: "bolt", Cause: abilitycomp.Cause{Caster: 1, Ability: "bolt"}, Owner: 1,
		Dir: physics2d.Vec2{X: 1}}
	b := projectilecomp.LaunchIntent{Shot: "bolt", Cause: abilitycomp.Cause{Caster: 2, Ability: "bolt"}, Owner: 2,
		Team: 1, From: physics2d.Vec2{Y: 5}, Dir: physics2d.Vec2{Y: 1}}
	teams := func(order ...projectilecomp.LaunchIntent) map[uint32]uint8 {
		fx := &emitter{}
		w := cardinal.NewTestWorld(t, withEmitter(register, fx))
		claim(t, w)
		for _, in := range order {
			emit(fx, in)
		}
		w.Tick()
		byID := map[uint32]uint8{}
		for _, s := range shots(t, w, "alice") {
			byID[s.EntityID] = s.Team
		}
		return byID
	}
	if ab, ba := teams(a, b), teams(b, a); len(ab) != 2 || !maps.Equal(ab, ba) {
		t.Fatalf("shots by entity ID: %v in one order, %v in the other; want the same two", ab, ba)
	}
}

// Features key their numbers by champion, ability, shot or effect ID, and the data plugin checks one
// file at a time. A hook that names a missing shot or effect kind does nothing, and a champion with no
// kit casts nothing.
func TestContentIDsLineUp(t *testing.T) {
	abilities := tuningOf[ability.AbilityTuning](t)
	shots, kinds := tuningOf[projectile.ProjectileTuning](t).Shots, tuningOf[effect.EffectTuning](t).Kinds
	for id, def := range abilities.Abilities {
		hook, err := hooks.Bind(def.Hook, def.Params, physics.ServerTickRateHz)
		if err != nil {
			t.Errorf("ability %q: %v", id, err)
			continue
		}
		for _, shot := range hook.Refs.Shots {
			if _, ok := shots[shot]; !ok {
				t.Errorf("ability %q launches unknown shot %q", id, shot)
			}
		}
		for _, kind := range hook.Refs.Effects {
			if _, ok := kinds[kind]; !ok {
				t.Errorf("ability %q applies unknown effect kind %q", id, kind)
			}
		}
	}
	champions := tuningOf[player.MovementTuning](t).Champions
	echoes, areas := tuningOf[echo.EchoTuning](t).Champions, tuningOf[area.AreaTuning](t).Kinds
	for id := range champions {
		if _, ok := abilities.Kits[id]; !ok {
			t.Errorf("champion %q has no kit", id)
		}
		if def, ok := echoes[id]; !ok {
			t.Errorf("champion %q has no Echo", id)
		} else if _, ok := areas[def.Area]; def.Area != "" && !ok {
			t.Errorf("champion %q's Echo has unknown area kind %q", id, def.Area)
		}
	}
	for id := range echoes {
		if _, ok := champions[id]; !ok {
			t.Errorf("Echo %q belongs to no champion", id)
		}
	}
	for id := range abilities.Kits {
		if _, ok := champions[id]; !ok {
			t.Errorf("kit %q belongs to no champion", id)
		}
	}
}

// --- Echo ---

// echoDelay returns the Echo delay of the champion dev Bodies play, in ticks.
func echoDelay(t *testing.T) int {
	t.Helper()
	return tuningOf[echo.EchoTuning](t).Champions[tuningOf[player.MovementTuning](t).DevChampion].DelayTicks()
}

// An Echo sits on its Body's spawn, then retraces the Body's path its champion's delay behind (test
// content: a champion whose Echo trails by 2 s).
func TestEchoRetracesItsBody(t *testing.T) {
	quick := withContent(t, map[string]string{
		"player/tuning.json":  `{"champions": {"quick": {"move_speed": 5, "radius": 0.5}}, "dev_champion": "quick"}`,
		"ability/tuning.json": `{"kits": {"quick": ["bolt"]}}`,
		"echo/tuning.json":    `{"champions": {"quick": {"delay_seconds": 2, "area": "anticamp"}}}`,
	})
	for _, c := range []struct {
		name  string
		setup func(*cardinal.World)
		delay int
	}{
		{"shipped", register, echoDelay(t)},
		{"quick", quick, 2 * physics.ServerTickRateHz},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := cardinal.NewTestWorld(t, c.setup)
			alice, _ := claim(t, w)
			spawn := yourBody(t, bodiesFor(t, w, "alice"))
			var path []player.BodyState
			for i := range c.delay + 20 {
				alice.move(math.Cos(float64(i)/7), math.Sin(float64(i)/7)) // a curve
				w.Tick()
				path = append(path, yourBody(t, bodiesFor(t, w, "alice")))
				want := spawn
				if i >= c.delay {
					want = path[i-c.delay]
				}
				if e := echoOf(t, w, "alice", spawn); e.X != want.X || e.Y != want.Y {
					t.Fatalf("tick %d: Echo at (%.4f, %.4f), want (%.4f, %.4f)", i, e.X, e.Y, want.X, want.Y)
				}
			}
		})
	}
}

// An Echo on its own Body hurts itself every tick, at its anti-camp area's rate (GDD §3). It is spared
// while its trail warms up, since it starts on its Body.
func TestEchoOnItsOwnBodyHurtsItself(t *testing.T) {
	w, _, _ := duel(t) // both stand on their Echoes
	delay := echoDelay(t)
	anticamp := tuningOf[area.AreaTuning](t).Kinds[tuningOf[echo.EchoTuning](t).Champions[tuningOf[player.MovementTuning](t).DevChampion].Area]
	if len(anticamp.Rules) == 0 {
		t.Fatal("the dev champion's Echo has no anti-camp rule")
	}
	perTick := anticamp.Rules[0].DamagePerSecond * physics.SecondsPerTick
	alice := yourBody(t, bodiesFor(t, w, "alice"))
	want := echoOf(t, w, "alice", alice).MaxHP
	for i := 1; i <= delay+5; i++ {
		w.Tick()
		if i >= delay {
			want -= perTick
		}
		if got := echoOf(t, w, "alice", alice).HP; got != want {
			t.Fatalf("tick %d of a %d-tick warmup: HP = %v, want %v", i, delay, got, want)
		}
	}
}

// Only an Echo's own Body hurts it: an enemy Body standing on it does nothing.
func TestEnemyBodyOnEchoDoesNotHurtIt(t *testing.T) {
	w, alice, bob := duel(t)
	delay := echoDelay(t)
	a, b := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	step := bodiesFor(t, w, "alice").You.MoveSpeed * physics.SecondsPerTick
	walk := int(math.Round((b.X - a.X) / step))
	moveFor(w, alice, 0, 1, 20)  // 5 m away, so her Echo stays on her spawn until the delay is over
	moveFor(w, bob, -1, 0, walk) // onto alice's Echo
	for i := 20 + walk + 3; i <= delay+5; i++ {
		w.Tick()
		e := echoOf(t, w, "alice", a)
		if i == delay { // the first tick anti-camp could bite
			aNow, bNow := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
			if math.Hypot(bNow.X-e.X, bNow.Y-e.Y) > 0.1 || math.Hypot(aNow.X-e.X, aNow.Y-e.Y) < 2 {
				t.Fatalf("Echo at (%.2f, %.2f), bob at (%.2f, %.2f), alice at (%.2f, %.2f): want bob on it, alice away",
					e.X, e.Y, bNow.X, bNow.Y, aNow.X, aNow.Y)
			}
		}
		if e.HP != e.MaxHP {
			t.Fatalf("tick %d: HP = %v under an enemy Body, want %v", i, e.HP, e.MaxHP)
		}
	}
}

// A champion's Echo can have no anti-camp, or another area (test content: a champion with none).
func TestChampionWithoutAntiCamp(t *testing.T) {
	w := cardinal.NewTestWorld(t, withContent(t, map[string]string{
		"player/tuning.json":  `{"champions": {"ghost": {"move_speed": 5, "radius": 0.5}}, "dev_champion": "ghost"}`,
		"ability/tuning.json": `{"kits": {"ghost": ["bolt"]}}`,
		"echo/tuning.json":    `{"champions": {"ghost": {"delay_seconds": 5, "area": ""}}}`,
	}))
	claim(t, w) // both stand on their Echoes
	alice := yourBody(t, bodiesFor(t, w, "alice"))
	flyFor(w, 5*physics.ServerTickRateHz+5)
	if e := echoOf(t, w, "alice", alice); e.HP != e.MaxHP {
		t.Fatalf("HP = %v on its own Body, want %v: this champion's Echo has no anti-camp", e.HP, e.MaxHP)
	}
}

// addArea builds an entity of side s at pos holding an area of kind kind, with a 0.5 m circle, acting at
// once. It's like an Echo.
func addArea(w *cardinal.TestWorld, kind string, s player.BodyState, pos physics2d.Vec2) cardinal.EntityID {
	type holderRow struct {
		Area      areacomp.Area
		Team      team.Team
		Owner     team.Owner
		Transform physics2d.Transform2D
		Velocity  physics2d.Velocity2D
		Body      physics2d.PhysicsBody2D
	}
	h := w.Create[holderRow]()
	h.Set(areacomp.Area{Kind: kind})
	h.Set(team.Team{ID: s.Team})
	h.Set(team.Owner{Body: cardinal.EntityID(s.EntityID)})
	h.Set(physics2d.Transform2D{Position: pos})
	h.Set(physics.NewBody2D(physics2d.BodyTypeManual, physics2d.ColliderShape{
		ShapeType: physics2d.ShapeTypeCircle, Radius: 0.5, CategoryBits: uint64(interact.Echo),
		MaskBits: uint64(interact.Shot | interact.Area),
	}))
	return h.ID()
}

// An area acts on what touches its circle, not on what only touches its bounding box.
func TestAreaActsOnWhatTouchesIt(t *testing.T) {
	w, _, _ := duel(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	addArea(w, "anticamp", a, physics2d.Vec2{X: a.X + 0.78, Y: a.Y + 0.78}) // its box overlaps her, its circle doesn't
	touching := addArea(w, "anticamp", a, physics2d.Vec2{X: a.X - 0.6, Y: a.Y - 0.6})
	w.Tick()
	hits := w.Emitted[combatcomp.DamageIntent]()
	if len(hits) != 1 || hits[0].Target != touching {
		t.Fatalf("damage %+v, want one, on the area %d that touches her", hits, touching)
	}
}

// Area rules see the tags a thing's effects give it (test content: an area that hurts its holder while
// its own Body is stunned).
func TestAreaRulesSeeTags(t *testing.T) {
	fx := &emitter{}
	w := cardinal.NewTestWorld(t, withEmitter(withContent(t, map[string]string{
		"area/tuning.json": `{"kinds": {"trap": {"rules": [
			{"kinds": ["body"], "require": ["stunned"], "to": "self", "damage_per_second": 20}]}}}`,
	}), fx))
	claim(t, w)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	trap := addArea(w, "trap", a, physics2d.Vec2{X: a.X, Y: a.Y})
	w.Tick()
	if n := len(w.Emitted[combatcomp.DamageIntent]()); n != 0 {
		t.Fatalf("%d damage while she isn't stunned, want 0", n)
	}
	fx.effect(a, effectcomp.EffectIntent{Kind: "stun", Ticks: 5})
	w.Tick() // the stun applies from the next tick
	w.Tick()
	if hits := w.Emitted[combatcomp.DamageIntent](); len(hits) != 1 || hits[0].Target != trap {
		t.Fatalf("damage %+v, want one on the trap %d while she is stunned", hits, trap)
	}
}

// --- Movement and walls ---

// addWall builds a static box wall that blocks Bodies and shots.
func addWall(w *cardinal.TestWorld, center, halfExtents physics2d.Vec2) {
	addMapBox(w, center, halfExtents, interact.Wall)
}

// addMapBox builds a static box of a map kind. Like every map shape it masks all bits, so each query's
// mask decides whether it stops there.
func addMapBox(w *cardinal.TestWorld, center, halfExtents physics2d.Vec2, kind interact.Kind) {
	type boxRow struct {
		Transform physics2d.Transform2D
		Velocity  physics2d.Velocity2D
		Body      physics2d.PhysicsBody2D
	}
	box := w.Create[boxRow]()
	box.Set(physics2d.Transform2D{Position: center})
	box.Set(physics.NewBody2D(physics2d.BodyTypeStatic, physics2d.ColliderShape{
		ShapeType:    physics2d.ShapeTypeBox,
		HalfExtents:  halfExtents,
		CategoryBits: uint64(kind),
		MaskBits:     math.MaxUint64,
	}))
}

// radiusOf returns the radius of b's collider circle.
func radiusOf(w *cardinal.TestWorld, b player.BodyState) float64 {
	return w.Entity(cardinal.EntityID(b.EntityID)).Get[physics2d.PhysicsBody2D]().Shapes.At(0).Radius
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

// --- Effects ---

// emitter emits queued system events in the next tick's PreUpdate, as a reactor would. An effect then
// applies from the tick after that; damage and heals apply that tick.
type emitter struct{ queue []func(*cardinal.World) }

func (e *emitter) Run(w *cardinal.World) {
	for _, emit := range e.queue {
		emit(w)
	}
	e.queue = nil
}

// emit queues ev for the next tick.
func emit[T interface{ Name() string }](e *emitter, ev T) {
	e.queue = append(e.queue, func(w *cardinal.World) { w.EmitSystemEvent(ev) })
}

// effect queues in for b.
func (e *emitter) effect(b player.BodyState, in effectcomp.EffectIntent) {
	in.Target = b.EntityID
	emit(e, in)
}

// withEmitter is setup plus fx, in PreUpdate so the resolvers see what it emits.
func withEmitter(setup func(*cardinal.World), fx *emitter) func(*cardinal.World) {
	return func(world *cardinal.World) {
		setup(world)
		world.RegisterSystem(fx, cardinal.WithHook(cardinal.PreUpdate))
	}
}

// fxDuel is duel, plus fx to emit effects, damage and heals.
func fxDuel(t *testing.T) (w *cardinal.TestWorld, alice, bob *client, fx *emitter) {
	t.Helper()
	fx = &emitter{}
	w = cardinal.NewTestWorld(t, withEmitter(register, fx))
	alice, bob = claim(t, w)
	return w, alice, bob, fx
}

// A phase lets a Body walk through walls, but the map edge still stops it.
func TestPhasePassesWallsNotBounds(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	addWall(w, physics2d.Vec2{X: a.X + 2, Y: a.Y}, physics2d.Vec2{X: 0.5, Y: 5})
	edge := a.X + 4.5
	addMapBox(w, physics2d.Vec2{X: edge + 0.5, Y: a.Y}, physics2d.Vec2{X: 0.5, Y: 20}, interact.Bounds)
	n := int(math.Ceil(4/(bodiesFor(t, w, "alice").You.MoveSpeed*physics.SecondsPerTick))) + 1 // past the edge
	fx.effect(a, effectcomp.EffectIntent{Kind: "phase", Ticks: uint64(n + 2)})
	w.Tick() // physics picks the shapes up; the phase applies from the next tick

	moveFor(w, alice, 1, 0, n) // through the wall and into the edge
	if got := yourBody(t, bodiesFor(t, w, "alice")).X + radiusOf(w, a); math.Abs(got-edge) > 0.01 {
		t.Fatalf("her edge stopped at x = %.4f, want the map edge at %.4f", got, edge)
	}
}

// A phase that ends inside a wall lands the Body on the nearest clear spot, instead of letting it walk
// out the far side.
func TestPhaseEndingInsideWallLands(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	face := a.X + 1.5
	addWall(w, physics2d.Vec2{X: face + 0.5, Y: a.Y}, physics2d.Vec2{X: 0.5, Y: 5})
	n := int(2 / (bodiesFor(t, w, "alice").You.MoveSpeed * physics.SecondsPerTick)) // to the middle of the wall
	fx.effect(a, effectcomp.EffectIntent{Kind: "phase", Ticks: uint64(n + 2)})
	w.Tick()

	moveFor(w, alice, 1, 0, n)
	if x := yourBody(t, bodiesFor(t, w, "alice")).X; x <= face || x >= face+1 {
		t.Fatalf("alice at x = %.4f, want her center inside the wall", x)
	}
	moveFor(w, alice, 0, 0, 2) // the phase runs out
	// She lands clear of the wall, on the side she came from, within one 0.25 m grid step of it.
	b := yourBody(t, bodiesFor(t, w, "alice"))
	if edge := b.X + radiusOf(w, a); !(edge < face && edge >= face-0.25) || b.Y != a.Y {
		t.Fatalf("landed at (%.4f, %.4f), want her edge within 0.25 m short of the wall at x = %.4f", b.X, b.Y, face)
	}
}

// Landing never crosses the map edge, even when the nearest clear spot is past it.
func TestLandingStaysInsideMapEdge(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	r := radiusOf(w, a)
	face := a.X + 1.5
	addWall(w, physics2d.Vec2{X: face + 1.5, Y: a.Y}, physics2d.Vec2{X: 1.5, Y: 5}) // 3 m deep
	edge := face + 3                                                                // right behind it, 0.2 m thick
	addMapBox(w, physics2d.Vec2{X: edge + 0.1, Y: a.Y}, physics2d.Vec2{X: 0.1, Y: 20}, interact.Bounds)
	n := int(math.Ceil(4/(bodiesFor(t, w, "alice").You.MoveSpeed*physics.SecondsPerTick))) + 1 // to the edge
	fx.effect(a, effectcomp.EffectIntent{Kind: "phase", Ticks: uint64(n + 2)})
	w.Tick()

	moveFor(w, alice, 1, 0, n) // through the wall, stopped by the edge with the wall around her
	if x := yourBody(t, bodiesFor(t, w, "alice")).X; math.Abs(x+r-edge) > 0.01 {
		t.Fatalf("alice at x = %.4f, want her edge on the map edge at %.4f", x, edge)
	}
	moveFor(w, alice, 0, 0, 2) // the phase runs out: past the edge is 1.25 m away, back out 3.25 m
	if b := yourBody(t, bodiesFor(t, w, "alice")); b.X+r >= face || b.Y != a.Y {
		t.Fatalf("landed at (%.4f, %.4f), want short of the wall at x = %.4f, inside the map", b.X, b.Y, face)
	}
}

func TestStunStopsInputMovement(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	fx.effect(yourBody(t, bodiesFor(t, w, "alice")), effectcomp.EffectIntent{Kind: "stun", Ticks: 5})
	alice.move(1, 0)
	w.Tick() // still moves: the stun applies from the next tick
	for range 5 {
		before := bodiesFor(t, w, "alice")
		alice.move(1, 0)
		w.Tick()
		requireMoved(t, before, bodiesFor(t, w, "alice"), 0, 0)
	}
	before := bodiesFor(t, w, "alice")
	alice.move(1, 0)
	w.Tick()
	requireMoved(t, before, bodiesFor(t, w, "alice"), 1, 0)
}

func TestSlowScalesInputSpeed(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	bob := yourBody(t, bodiesFor(t, w, "bob")).EntityID
	slow := effectcomp.EffectIntent{Kind: "slow", Ticks: 20, Value: -4000, Source: bob}
	fx.effect(yourBody(t, bodiesFor(t, w, "alice")), slow)
	w.Tick()
	before := bodiesFor(t, w, "alice")
	alice.move(1, 0)
	w.Tick()
	after := bodiesFor(t, w, "alice")
	step := after.You.MoveSpeed * after.TickSeconds
	if moved := yourBody(t, after).X - yourBody(t, before).X; math.Abs(moved-0.6*step) > 1e-9 {
		t.Fatalf("a 40%% slow moved her %.4f, want %.4f", moved, 0.6*step)
	}
}

// HP stays between 0 and MaxHP. Damage lands first, then heals, and deaths only after both, so a
// same-tick heal saves a target if it covers the overkill.
func TestHealthStaysBetweenZeroAndMaxHP(t *testing.T) {
	w, _, _, fx := fxDuel(t)
	bobs := func() echo.EchoState { return echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))) }
	e := bobs()
	target := cardinal.EntityID(e.EntityID)

	emit(fx, combatcomp.DamageIntent{Target: target, Amount: 30})
	emit(fx, combatcomp.HealIntent{Target: target, Amount: 100})
	w.Tick()
	if got := bobs().HP; got != e.MaxHP {
		t.Fatalf("HP = %v after 30 damage and a 100 heal, want MaxHP %v", got, e.MaxHP)
	}

	emit(fx, combatcomp.HealIntent{Target: target, Amount: e.MaxHP}) // emitted first, lands second
	emit(fx, combatcomp.DamageIntent{Target: target, Amount: e.MaxHP + 10})
	w.Tick()
	if got := bobs().HP; got != e.MaxHP-10 {
		t.Fatalf("HP = %v after a lethal hit and a heal in one tick, want %v", got, e.MaxHP-10)
	}

	emit(fx, combatcomp.DamageIntent{Target: target, Amount: e.MaxHP})
	mine := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice")))
	emit(fx, combatcomp.HealIntent{Target: cardinal.EntityID(mine.EntityID), Amount: -mine.MaxHP}) // a bad heal
	w.Tick()
	for other := range sentTo[echo.EchoSync](t, w, "alice").Echoes.Values() {
		if other.EntityID == e.EntityID || other.EntityID == mine.EntityID {
			t.Fatalf("Echo %d is alive at %v HP; at 0 or below it dies", other.EntityID, other.HP)
		}
	}
}

// dashParams are a dash hook's params.
type dashParams struct {
	Distance float64 `json:"distance"`
	Seconds  float64 `json:"seconds"`
}

// Ticks converts Seconds to ticks.
func (d dashParams) Ticks() uint64 {
	return uint64(math.Round(d.Seconds * float64(physics.ServerTickRateHz)))
}

// dash returns the kit slot of the dev champion's first dash, and its params.
func dash(t *testing.T) (int, dashParams) {
	t.Helper()
	abilities := tuningOf[ability.AbilityTuning](t).Abilities
	for slot, id := range kit(t) {
		if def := abilities[id]; def.Hook == "dash" {
			var d dashParams
			if err := json.Unmarshal(def.Params, &d); err != nil {
				t.Fatal(err)
			}
			return slot, d
		}
	}
	t.Fatal("the kit has no dash")
	return 0, dashParams{}
}

// A dash goes its distance along the aim, from the tick after the cast, and then stops.
func TestDashGoesItsDistanceAlongAim(t *testing.T) {
	w, alice, _ := duel(t)
	slot, d := dash(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	alice.cast(slot, 0, 1)
	w.Tick()
	if b := yourBody(t, bodiesFor(t, w, "alice")); b.X != a.X || b.Y != a.Y {
		t.Fatalf("moved to (%.4f, %.4f) on the cast tick", b.X, b.Y)
	}
	flyFor(w, int(d.Ticks())+2)
	if b := yourBody(t, bodiesFor(t, w, "alice")); math.Abs(b.Y-a.Y-d.Distance) > 1e-9 || b.X != a.X {
		t.Fatalf("dashed to (%.4f, %.4f), want %.1f m along +y from (%.4f, %.4f)", b.X, b.Y, d.Distance, a.X, a.Y)
	}
}

// A dash replaces input while it lasts, walls still stop it, and it doesn't leak into the coast.
func TestDashIgnoresInputAndStopsAtWall(t *testing.T) {
	w, alice, _ := duel(t)
	slot, d := dash(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	r := radiusOf(w, a)
	face := a.X + r + d.Distance/2 // halfway along the dash
	addWall(w, physics2d.Vec2{X: face + 0.5, Y: a.Y}, physics2d.Vec2{X: 0.5, Y: 5})
	alice.cast(slot, 1, 0)
	w.Tick() // physics picks the wall up; the dash runs from the next tick

	for range d.Ticks() {
		alice.move(0, 1) // ignored while dashing
		w.Tick()
	}
	dashed := bodiesFor(t, w, "alice")
	if b := yourBody(t, dashed); math.Abs(b.X+r-face) > 0.01 || b.Y != a.Y {
		t.Fatalf("dash ended at (%.4f, %.4f), want her edge on the wall at x = %.4f, y unchanged", b.X, b.Y, face)
	}
	w.Tick() // no frame: she coasts on her last one at her own speed, not the dash's
	requireMoved(t, dashed, bodiesFor(t, w, "alice"), 0, 1)
}

// A stun ends the Body's own dash, which doesn't come back when the stun is over.
func TestStunEndsDash(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	slot, d := dash(t)
	if d.Ticks() < 3 {
		t.Fatalf("a %d-tick dash is over before the stun lands", d.Ticks())
	}
	a := yourBody(t, bodiesFor(t, w, "alice"))
	alice.cast(slot, 0, 1)
	w.Tick() // the dash runs from the next tick
	w.Tick()
	fx.effect(a, effectcomp.EffectIntent{Kind: "stun", Ticks: 5})
	w.Tick() // the stun applies from the next tick
	flyFor(w, int(d.Ticks())+5)
	step := d.Distance / float64(d.Ticks())
	if y := yourBody(t, bodiesFor(t, w, "alice")).Y; math.Abs(y-(a.Y+2*step)) > 1e-9 {
		t.Fatalf("alice at y = %.4f, want %.4f: 2 ticks of dash, then none", y, a.Y+2*step)
	}
}

func TestStunnedBodyCannotCast(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	body := yourBody(t, bodiesFor(t, w, "alice"))
	fx.effect(body, effectcomp.EffectIntent{Kind: "stun", Ticks: 5})
	w.Tick() // the stun applies from the next tick
	alice.fire(0, 1)
	w.Tick()
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("a stunned Body fired %d shots", n)
	}
	if ready, _ := readyTick(t, w, "alice", body.EntityID); ready != 0 {
		t.Fatalf("ReadyTick = %d, want 0: no cooldown spent", ready)
	}
}

// An effect of an unknown kind is dropped, and doesn't stop the others sent with it.
func TestUnknownEffectKindIsDropped(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	fx.effect(a, effectcomp.EffectIntent{Kind: "nope", Ticks: 5})
	fx.effect(a, effectcomp.EffectIntent{Kind: "stun", Ticks: 5})
	w.Tick() // the stun applies from the next tick
	before := bodiesFor(t, w, "alice")
	alice.move(1, 0)
	w.Tick()
	requireMoved(t, before, bodiesFor(t, w, "alice"), 0, 0)
}

// Never assume the caster: every ability, cast by a Body or by an Echo, sends intents that carry the
// caster as the cause and start from it, for the caster's owner.
func TestAnyCasterCastsEveryAbility(t *testing.T) {
	w, _, _, fx := fxDuel(t)
	body := yourBody(t, bodiesFor(t, w, "alice"))
	casters := map[string]cardinal.EntityID{
		"Body": cardinal.EntityID(body.EntityID),
		"Echo": cardinal.EntityID(echoOf(t, w, "alice", body).EntityID),
	}
	// where returns where the caster was this tick.
	where := func(caster string) physics2d.Vec2 {
		if caster == "Body" {
			b := yourBody(t, bodiesFor(t, w, "alice"))
			return physics2d.Vec2{X: b.X, Y: b.Y}
		}
		e := echoOf(t, w, "alice", body)
		return physics2d.Vec2{X: e.X, Y: e.Y}
	}
	for id := range tuningOf[ability.AbilityTuning](t).Abilities {
		for caster, cid := range casters {
			emit(fx, abilitycomp.AbilityCast{Cause: abilitycomp.Cause{Caster: cid, Ability: id}, AimX: 0.6, AimY: 0.8})
			w.Tick()
			launches, effects := w.Emitted[projectilecomp.LaunchIntent](), w.Emitted[effectcomp.EffectIntent]()
			if len(launches)+len(effects) == 0 {
				t.Errorf("%s cast by her %s sent no intent", id, caster)
			}
			from := where(caster)
			for _, l := range launches {
				if l.Cause.Caster != cid || l.Owner != cardinal.EntityID(body.EntityID) || l.Team != body.Team || l.From != from {
					t.Errorf("%s cast by her %s: %+v, want it from %v, caused by %d, for her Body %d",
						id, caster, l, from, cid, body.EntityID)
				}
			}
			for _, in := range effects {
				if in.Source != uint32(cid) {
					t.Errorf("%s cast by her %s: effect %+v, want its source %d", id, caster, in, cid)
				}
			}
		}
	}
}

// The same script twice gives the same state, events and system events after every tick.
func TestEffectsAreDeterministic(t *testing.T) {
	var fx *emitter // the current run's: each run calls setup, then script
	cardinal.RequireDeterministic(t, func(world *cardinal.World) {
		fx = &emitter{}
		withEmitter(register, fx)(world)
	}, func(w *cardinal.TestWorld) {
		alice, bob := claim(t, w)
		a, b := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
		aID, bID := a.EntityID, b.EntityID
		addWall(w, physics2d.Vec2{X: a.X + 2, Y: a.Y}, physics2d.Vec2{X: 0.5, Y: 5})
		fx.effect(a, effectcomp.EffectIntent{Kind: "phase", Ticks: 12}) // ends inside the wall
		fx.effect(a, effectcomp.EffectIntent{Kind: "slow", Ticks: 30, Value: -2500, Source: bID})
		slot, _ := dash(t)
		bob.cast(slot, -0.6, 0.8)
		w.Tick()
		for i := range 30 {
			if i == 3 {
				fx.effect(b, effectcomp.EffectIntent{Kind: "stun", Ticks: 4, Source: aID})
			}
			alice.move(1, 0.1)
			bob.move(-1, 0)
			w.Tick()
		}
	})
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

// --- Prediction ---

// predictionStep is one tick of TestPredictionMatchesServer's run: the body_sync before it, the frame
// the server applied, and what the predictor gave after it, as predictor.wasm returns it.
type predictionStep struct {
	Sync  player.BodySync
	Frame playercomp.InputFrame
	X, Y  float64          // your Body
	Shots []shotPrediction // your shots, in the predictor's order
	Kit   kitPrediction
}

// shotPrediction is one of your shots, as predictor.wasm's shots() returns it.
type shotPrediction struct {
	Type                       string
	X, Y, PrevX, PrevY, Radius float64
}

// kitPrediction is your kit, as predictor.wasm's kit() returns it: ticks are JS numbers there.
type kitPrediction struct {
	Tick  float64
	Slots []kitSlot
}

type kitSlot struct {
	Ability   string
	ReadyTick float64
}

// predictFrom is the predictor's start from a body_sync, as client/src/game/player/predictor.ts builds
// it. s goes through MessagePack first, as it does to a client.
func predictFrom(t *testing.T, s player.BodySync) predict.State {
	t.Helper()
	raw, err := msgpack.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var got player.BodySync
	if err := msgpack.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	b, you := yourBody(t, got), got.You
	state := predict.State{
		Tick: got.Tick, Body: you.Body, Team: b.Team, Center: box2d.Vec2{X: b.X, Y: b.Y}, Radius: b.Radius,
		MoveSpeed: you.MoveSpeed, Champion: you.Champion, Cooldowns: slices.Collect(you.Cooldowns.Values()),
		Effects: slices.Collect(you.Effects.Values()), EffectsNext: you.EffectsNext,
	}
	for shot := range you.Shots.Values() {
		state.Shots = append(state.Shots, predict.Shot{Type: shot.Type, Flight: flight.Flight{
			Pos:      box2d.Vec2{X: shot.X, Y: shot.Y},
			Velocity: box2d.Vec2{X: shot.VelX, Y: shot.VelY}, Radius: shot.Radius, Remaining: shot.Remaining,
		}})
	}
	return state
}

// requirePredicted fails unless p's state is the server's in after: her Body, cooldowns and shots, and
// her effects unless something else put one on her this tick.
func requirePredicted(t *testing.T, at string, p *predict.Predictor, after player.BodySync, effects bool) {
	t.Helper()
	got, want := p.State(), predictFrom(t, after)
	if got.Center != want.Center {
		t.Fatalf("%s: predicted her Body at %v, the server has %v", at, got.Center, want.Center)
	}
	if !slices.Equal(got.Cooldowns, want.Cooldowns) {
		t.Fatalf("%s: predicted cooldowns %+v, the server has %+v", at, got.Cooldowns, want.Cooldowns)
	}
	if effects && (!slices.Equal(got.Effects, want.Effects) || got.EffectsNext != want.EffectsNext) {
		t.Fatalf("%s: predicted effects %+v (next %d), the server has %+v (next %d)",
			at, got.Effects, got.EffectsNext, want.Effects, want.EffectsNext)
	}
	// The server lists shots in no set order, and has no Prev.
	flights := func(shots []predict.Shot) []predict.Shot {
		out := make([]predict.Shot, len(shots))
		for i, s := range shots {
			out[i] = predict.Shot{Type: s.Type, Flight: s.Flight}
		}
		slices.SortFunc(out, func(a, b predict.Shot) int {
			return cmp.Or(cmp.Compare(a.Pos.X, b.Pos.X), cmp.Compare(a.Pos.Y, b.Pos.Y))
		})
		return out
	}
	if g, w := flights(got.Shots), flights(want.Shots); !slices.Equal(g, w) {
		t.Fatalf("%s: predicted shots %+v, the server has %+v", at, g, w)
	}
}

// record is p's state as predictor.wasm returns it, for testdata/prediction.msgpack.
func record(p *predict.Predictor, sync player.BodySync, f playercomp.InputFrame) predictionStep {
	s := p.State()
	step := predictionStep{
		Sync: sync, Frame: f, X: s.Center.X, Y: s.Center.Y,
		Shots: []shotPrediction{}, Kit: kitPrediction{Tick: float64(s.Tick), Slots: []kitSlot{}},
	}
	for _, shot := range s.Shots {
		step.Shots = append(step.Shots, shotPrediction{
			Type: shot.Type, X: shot.Pos.X, Y: shot.Pos.Y, PrevX: shot.Prev.X, PrevY: shot.Prev.Y, Radius: shot.Radius,
		})
	}
	for _, c := range p.Kit() {
		step.Kit.Slots = append(step.Kit.Slots, kitSlot{Ability: c.Ability, ReadyTick: float64(c.ReadyTick)})
	}
	return step
}

// The client predicts its Body with internal/predict, from what body_sync sends: its moves, its casts
// and their cooldowns, its dash, and its shots. Started from any tick's state and stepped by the frames
// after it, it lands on the server's bits, through slows, stuns, forced moves, held buttons, bad frames
// and a shot flying out its range. Effects others put on her surprise it, and come with the next
// state. testdata/prediction.msgpack keeps the run, so the client checks that its wasm build lands
// there too (client/src/game/player/predictor.test.ts).
func TestPredictionMatchesServer(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	rules, err := predict.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	p := predict.New(rules)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	from := yourBody(t, bodiesFor(t, w, "bob")).EntityID
	dashSlot, _ := dash(t)
	_, bolt := primary(t)
	if steps := int(bolt.Range / (bolt.Speed * physics.SecondsPerTick)); steps >= 39 {
		t.Fatalf("a bolt flies %d ticks, past the run", steps)
	}
	// What bob puts on her, by step. Each applies from the tick after it.
	surprises := map[int]effectcomp.EffectIntent{
		2: {Kind: "slow", Ticks: 9, Value: -4000, Source: from},
		6: {Kind: "stun", Ticks: 3, Source: from}, // ends her dash
	}
	moves := [][2]float64{{1, 0}, {0.3, -0.9}, {-1, 1}, {0, 0}, {math.NaN(), 1}, {-0.6, -0.8}, {3, 4}, {1e-12, 0}}
	// She holds fire for six ticks, aiming up and left, away from bob: it casts once, then cools down.
	// She also dashes on the fourth.
	frameAt := func(i int) playercomp.InputFrame {
		m := moves[i%len(moves)]
		f := playercomp.InputFrame{MoveX: m[0], MoveY: m[1], AimX: -0.6, AimY: 0.8}
		switch {
		case i == 3:
			f.Buttons = 1 | 1<<dashSlot
		case i < 6:
			f.Buttons = 1
		}
		return f
	}

	// One frame from each tick's state, as when the server acks every frame.
	var run []predictionStep
	for i := range 40 {
		e, surprised := surprises[i]
		if surprised {
			fx.effect(a, e)
		}
		before := bodiesFor(t, w, "alice")
		f := frameAt(i)
		alice.send(f)
		w.Tick()
		f.Seq = alice.seq
		p.Reset(predictFrom(t, before))
		p.Step(predict.Frame(f))
		requirePredicted(t, fmt.Sprintf("step %d", i), p, bodiesFor(t, w, "alice"), !surprised)
		run = append(run, record(p, before, f))
	}
	if len(run[0].Shots) != 1 || len(run[39].Shots) != 0 {
		t.Fatalf("predicted %d shots after her first frame and %d at the end, want her one bolt, then none",
			len(run[0].Shots), len(run[39].Shots))
	}

	// Ten frames from one state, as when acks lag. A stun and a slow from bob are in that state and end
	// partway, and her bolt is ready again. She holds fire: the stun stops her first two frames, a bad aim
	// and no aim the next two, and the fifth casts.
	fx.effect(a, effectcomp.EffectIntent{Kind: "slow", Ticks: 5, Value: -2500, Source: from})
	fx.effect(a, effectcomp.EffectIntent{Kind: "stun", Ticks: 2, Source: from})
	alice.move(1, 0)
	w.Tick()
	p.Reset(predictFrom(t, bodiesFor(t, w, "alice")))
	aims := [][2]float64{{-1, 0.25}, {-1, 0.25}, {math.NaN(), 1}, {0, 0}, {-1, 0.25}}
	for i := range 10 {
		m, aim := moves[i%len(moves)], aims[min(i, len(aims)-1)]
		f := playercomp.InputFrame{MoveX: m[0], MoveY: m[1], AimX: aim[0], AimY: aim[1], Buttons: 1}
		alice.send(f)
		w.Tick()
		p.Step(predict.Frame(f))
		requirePredicted(t, fmt.Sprintf("replayed frame %d", i+1), p, bodiesFor(t, w, "alice"), true)
		if shots := len(p.State().Shots); shots != 0 && i < 4 || shots != 1 && i >= 4 {
			t.Fatalf("replayed frame %d: %d shots predicted, want her bolt from frame 5 only", i+1, shots)
		}
	}

	// The recording must still predict its own results, so a change to the sim asks for a new one.
	path := filepath.Join("testdata", "prediction.msgpack")
	if *update {
		raw, err := msgpack.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var recorded []predictionStep
	if err := msgpack.Unmarshal(raw, &recorded); err != nil || len(recorded) == 0 {
		t.Fatalf("%s holds no run (%v)", path, err)
	}
	for i, s := range recorded {
		p.Reset(predictFrom(t, s.Sync))
		p.Step(predict.Frame(s.Frame))
		got := record(p, s.Sync, s.Frame)
		if got.X != s.X || got.Y != s.Y || !slices.Equal(got.Shots, s.Shots) || got.Kit.Tick != s.Kit.Tick ||
			!slices.Equal(got.Kit.Slots, s.Kit.Slots) {
			t.Fatalf("%s step %d: predicted (%v, %v), shots %+v, kit %+v; recorded (%v, %v), %+v, %+v. If the sim "+
				"changed on purpose, run `go test ./shards/game -run TestPredictionMatchesServer -update`",
				path, i, got.X, got.Y, got.Shots, got.Kit, s.X, s.Y, s.Shots, s.Kit)
		}
	}
}

// --- Shared sim code ---

// sharedCore is the code the client predictor runs too, box2d included.
var sharedCore = []string{"./internal/physics/mover", "./internal/physics/timestep", "./internal/effect/status",
	"./internal/interact", "./internal/ability/hooks", "./internal/ability/cast", "./internal/projectile/flight",
	"./internal/player/motion", "./internal/predict", "github.com/argus-labs/world-engine/pkg/box2d"}

// exactMath is the math functions the shared core may call: their results are exact, so no target
// rounds them differently. The rest of math isn't scanned, and some of it fuses (math.Hypot on arm64).
var exactMath = map[string]bool{"remainder": true, "mod": true, "modf": true, "floor": true, "ceil": true,
	"trunc": true, "round": true, "roundtoeven": true, "frexp": true, "ldexp": true}

// The shared core imports no Cardinal (a component leaf pulls in about 737 packages), and builds with
// no fused multiply-add on targets that have one. An FMA rounds a*b+c once, so it gives other bits
// than a target that can't fuse, like wasm. Fix a hit by rounding the product: float64(a*b). It calls
// only exactMath of the math package: use box2d's FMA-free helpers (box2d.Length) for the rest.
func TestSharedCoreIsDeterministic(t *testing.T) {
	deps, err := exec.Command("go", append([]string{"list", "-deps"}, sharedCore...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, deps)
	}
	for dep := range strings.Lines(string(deps)) {
		if strings.HasPrefix(dep, "github.com/argus-labs/world-engine/pkg/cardinal") {
			t.Errorf("the shared core imports %s", strings.TrimSpace(dep))
			break
		}
	}
	fma := regexp.MustCompile(`\bV?FN?M(ADD|SUB)[0-9A-Z]*\b`) // FMADDD on arm64, VFMADD231SD on amd64/v3
	mathCall := regexp.MustCompile(`\bCALL\s+math\.(\w+)`)
	for _, target := range [][]string{{"GOOS=linux", "GOARCH=arm64"}, {"GOOS=linux", "GOARCH=amd64", "GOAMD64=v3"},
		{"GOOS=js", "GOARCH=wasm"}} {
		build := exec.Command("go", append([]string{"build", "-gcflags=-S"}, sharedCore...)...) // -S prints assembly
		build.Env = append(build.Environ(), target...)
		asm, err := build.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", target, err, asm)
		}
		for line := range strings.Lines(string(asm)) {
			if fma.MatchString(line) {
				t.Errorf("%v: fused multiply-add: %s", target, strings.TrimSpace(line))
			}
			if m := mathCall.FindStringSubmatch(line); m != nil && !exactMath[strings.TrimPrefix(strings.ToLower(m[1]), "arch")] {
				t.Errorf("%v: calls math.%s, which can round differently per target: %s", target, m[1], strings.TrimSpace(line))
			}
		}
	}
}

// --- Wire format ---

var update = flag.Bool("update", false, "rewrite testdata from TestWireFixtures and TestPredictionMatchesServer")

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
			You: player.YouState{
				Body: 1, InputSession: 4294967295, InputAck: 42, MoveSpeed: 5, Champion: "starter",
				Cooldowns: cardinal.SliceOf(
					cast.Cooldown{Ability: "bolt", ReadyTick: 9007199254740993},
					cast.Cooldown{Ability: "dash", ReadyTick: 0},
				),
				Effects: cardinal.SliceOf(
					status.Entry{Kind: "slow", Tags: interact.CC, Stat: status.MoveSpeed, Value: -4000, Source: 2,
						Start: 9007199254740993, End: 18446744073709551615, Serial: 1},
					status.Entry{Kind: "dash", Tags: interact.EndsOnCC, Pass: interact.Wall, VelX: -12.5, VelY: 16.25,
						Source: 1, Start: 7, End: 13, Serial: 2},
				),
				EffectsNext: 4294967295,
				Shots: cardinal.SliceOf(player.ShotState{
					Type: "bolt", X: -4.25, Y: 1.5, VelX: 9, VelY: -12, Radius: 0.15, Remaining: 17.75,
				}),
			},
			Bodies: cardinal.SliceOf(
				player.BodyState{EntityID: 1, PlayerID: 1, Nickname: "Player One", Team: 0, X: 1.5, Y: -2, Radius: 0.5},
				player.BodyState{EntityID: 2, PlayerID: 2, Nickname: "Échø ✦", Team: 1, X: -0.25, Y: 3.75, Radius: 0.75},
			),
		}},
		{"echo_sync", echo.EchoSync{Tick: 7, Echoes: cardinal.SliceOf(
			echo.EchoState{EntityID: 3, PlayerID: 1, Team: 0, X: -4.5, Y: 0.5, HP: 62.5, MaxHP: 100},
		)}},
		{"ability_sync", ability.AbilitySync{Tick: 7, Abilities: cardinal.SliceOf(
			ability.AbilityState{Body: 1, Slot: 0, Ability: "bolt", ReadyTick: 18446744073709551615},
		)}},
		{"projectile_sync", projectile.ProjectileSync{Tick: 8, Projectiles: cardinal.SliceOf(
			projectile.ProjectileState{EntityID: 9, Type: "bolt", Caster: 4294967295, Team: 1, X: 0.125, Y: -7.5, Radius: 0.15},
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
