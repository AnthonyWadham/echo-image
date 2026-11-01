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
	"reflect"
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

// send numbers f and sends it, and returns it numbered.
func (c *client) send(f playercomp.InputFrame) playercomp.InputFrame {
	c.seq++
	f.Seq = c.seq
	c.w.Command(c.id, input(1, f))
	return f
}

func (c *client) move(x, y float64) { c.send(playercomp.InputFrame{MoveX: x, MoveY: y}) }

// cast holds the button for kit slot, aiming (aimX, aimY), and returns the frame's Seq.
func (c *client) cast(slot int, aimX, aimY float64) uint32 {
	return c.send(playercomp.InputFrame{AimX: aimX, AimY: aimY, Buttons: 1 << slot}).Seq
}

// fire casts kit slot 0.
func (c *client) fire(aimX, aimY float64) uint32 { return c.cast(0, aimX, aimY) }

// claim sends a frame with hit claim h, as c drew things back ticks before the tick it applies at.
func (c *client) claim(h playercomp.HitClaim, back float64) {
	c.send(playercomp.InputFrame{ViewTick: float64(c.w.TickHeight()) - back, Claims: cardinal.SliceOf(h)})
}

// fireClaiming fires kit slot 0 aiming (aimX, aimY), and claims in the same frame that the shot hit
// target at once, as c drew it back ticks before.
func (c *client) fireClaiming(t *testing.T, aimX, aimY float64, target uint32, back float64) {
	t.Helper()
	h := playercomp.HitClaim{Seq: c.seq + 1, Ability: kit(t)[0], Target: target}
	c.send(playercomp.InputFrame{AimX: aimX, AimY: aimY, Buttons: 1, ViewTick: float64(c.w.TickHeight()) - back,
		Claims: cardinal.SliceOf(h)})
}

// claimOn is c's claim that its one shot in flight hit target, keyed as body_sync shows the shot.
func claimOn(t *testing.T, w *cardinal.TestWorld, c *client, target uint32) playercomp.HitClaim {
	t.Helper()
	shots := bodiesFor(t, w, c.id).You.Shots
	if shots.Len() != 1 {
		t.Fatalf("%s has %d shots in flight, want 1", c.id, shots.Len())
	}
	s := shots.At(0)
	return playercomp.HitClaim{Seq: s.Seq, Ability: s.Ability, Launch: s.Launch, Target: target}
}

// firstTouch is the step, counted from the one c's shot left on, whose path first touches a circle of
// radius r that lies dist from where it left, along its path. Its speed and size come from body_sync.
func firstTouch(t *testing.T, w *cardinal.TestWorld, c *client, dist, r float64) int {
	t.Helper()
	s := bodiesFor(t, w, c.id).You.Shots.At(0)
	return int((dist - r - s.Radius) / (math.Hypot(s.VelX, s.VelY) * physics.SecondsPerTick))
}

// claimAt ticks to step k after the one the shot left on, with c claiming h only in that step's frame,
// as its predictor would: the step whose path first touches the target.
func claimAt(w *cardinal.TestWorld, c *client, h playercomp.HitClaim, back float64, k int) {
	flyFor(w, k-1)
	c.claim(h, back)
	w.Tick()
}

// drawLag is how far behind the server a client draws, in ticks: its interpolation delay and some ping.
const drawLag = 2.5

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

// fireAtEcho has alice fire at bob's Echo and checks the shot left. It returns the step after that
// whose path first touches the Echo, short of the shot's range, and her claim that it hit the Echo.
func fireAtEcho(t *testing.T, w *cardinal.TestWorld, alice *client) (int, playercomp.HitClaim) {
	t.Helper()
	from := yourBody(t, bodiesFor(t, w, "alice"))
	to := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	alice.fire(to.X-from.X, to.Y-from.Y)
	w.Tick()
	if n := len(shots(t, w, "alice")); n != 1 {
		t.Fatalf("%d shots after firing, want 1", n)
	}
	_, shot := primary(t)
	dist := math.Hypot(to.X-from.X, to.Y-from.Y)
	if dist >= shot.Range-2*shot.Speed*physics.SecondsPerTick {
		t.Fatalf("bob's Echo is %.1f away, beyond the shot's range", dist)
	}
	return firstTouch(t, w, alice, dist, to.Radius), claimOn(t, w, alice, to.EntityID)
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

// shots returns the shots in flight that viewer sees.
func shots(t *testing.T, w *cardinal.TestWorld, viewer string) []projectile.ProjectileState {
	t.Helper()
	all := slices.Collect(sentTo[projectile.ProjectileSync](t, w, viewer).Projectiles.Values())
	return slices.DeleteFunc(all, func(s projectile.ProjectileState) bool { return s.Ended })
}

// endedShots returns the shots viewer saw end this tick, each where it ended.
func endedShots(t *testing.T, w *cardinal.TestWorld, viewer string) []projectile.ProjectileState {
	t.Helper()
	all := slices.Collect(sentTo[projectile.ProjectileSync](t, w, viewer).Projectiles.Values())
	return slices.DeleteFunc(all, func(s projectile.ProjectileState) bool { return !s.Ended })
}

// flyUntilEnded ticks until viewer sees shots end, at most n ticks, and returns where they ended.
func flyUntilEnded(t *testing.T, w *cardinal.TestWorld, viewer string, n int) []projectile.ProjectileState {
	t.Helper()
	for range n {
		w.Tick()
		if ended := endedShots(t, w, viewer); len(ended) > 0 {
			return ended
		}
	}
	return nil
}

// touching reports whether shot s ended touching a circle of radius r at (x, y). Box2D stops a cast a
// slop short.
func touching(s projectile.ProjectileState, x, y, r float64) bool {
	return math.Abs(math.Hypot(s.X-x, s.Y-y)-(s.Radius+r)) <= 2*box2d.LinearSlop && s.Until > 0 && s.Until <= 1
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
	// Her client draws it from her predictor instead, by Caster. It flies the whole tick.
	if s := shots(t, w, "bob")[0]; s.Caster != body || s.Until != 1 {
		t.Fatalf("the shot's Caster = %d and Until %v, want alice's Body %d and 1", s.Caster, s.Until, body)
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

// alice's client claims her shot hit bob's Echo where it drew it: the server checks the claim, stops
// the shot there, and damages the Echo. Everyone is sent the shot once more, touching the Echo.
func TestShotDamagesEnemyEcho(t *testing.T) {
	w, alice, bob := duel(t)
	// bob steps off his Echo, which stays at his spawn while its trail warms up.
	moveFor(w, bob, 0, 1, 10)

	k, h := fireAtEcho(t, w, alice)
	claimAt(w, alice, h, drawLag, k)

	bobBody := yourBody(t, bodiesFor(t, w, "bob"))
	got := echoOf(t, w, "alice", bobBody)
	if _, shot := primary(t); got.HP != got.MaxHP-shot.Damage {
		t.Fatalf("bob's Echo HP = %v, want %v", got.HP, got.MaxHP-shot.Damage)
	}
	if ended := endedShots(t, w, "bob"); len(ended) != 1 || !touching(ended[0], got.X, got.Y, got.Radius) {
		t.Fatalf("bob saw shots end at %+v, want one touching his Echo at (%v, %v)", ended, got.X, got.Y)
	}
	if mine := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice"))); mine.HP != mine.MaxHP {
		t.Fatalf("alice's own Echo took damage: HP %v", mine.HP)
	}
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("%d shots left after the hit, want 0", n)
	}
}

// A shot that kills an Echo (test content: Echoes with one shot of HP): everyone gets it once more, at
// HP 0, alive until the shot reached it. Then it's gone.
func TestKilledEchoLastsUntilTheShotReachesIt(t *testing.T) {
	_, shot := primary(t)
	w := cardinal.NewTestWorld(t, withContent(t, map[string]string{
		"echo/tuning.json": fmt.Sprintf(`{"max_hp": %v}`, shot.Damage),
	}))
	alice, bob := claim(t, w)
	moveFor(w, bob, 0, 1, 10) // off his Echo
	k, h := fireAtEcho(t, w, alice)
	claimAt(w, alice, h, drawLag, k)

	ended := endedShots(t, w, "bob")
	got := echoOf(t, w, "bob", yourBody(t, bodiesFor(t, w, "bob")))
	if len(ended) != 1 || got.HP != 0 || got.Until != ended[0].Until || ended[0].Until >= 1 ||
		!touching(ended[0], got.X, got.Y, got.Radius) {
		t.Fatalf("bob got his dying Echo %+v and shots ending %+v, want it at 0 HP until the shot touched it", got, ended)
	}
	w.Tick()
	for e := range sentTo[echo.EchoSync](t, w, "bob").Echoes.Values() {
		if e.EntityID == got.EntityID {
			t.Fatalf("bob's Echo is still sent a tick after it died: %+v", e)
		}
	}
	if w.Entity(cardinal.EntityID(got.EntityID)).Has[combatcomp.Health]() {
		t.Fatal("bob's Echo is still in the world a tick after it died")
	}
}

// A shot's damage carries the cast that fired it, and credits the Body that cast it.
func TestShotDamageCarriesItsCast(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, 10) // off his Echo
	k, h := fireAtEcho(t, w, alice)
	casts := w.Emitted[abilitycomp.AbilityCast]()
	if len(casts) != 1 {
		t.Fatalf("%d casts, want 1", len(casts))
	}
	claimAt(w, alice, h, drawLag, k)
	hits := w.Emitted[combatcomp.DamageIntent]()
	target := cardinal.EntityID(echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))).EntityID)
	if len(hits) != 1 || hits[0].Target != target || hits[0].Credit != casts[0].Caster || hits[0].Cause != casts[0].Cause {
		t.Fatalf("damage %+v, want one on bob's Echo %d, credited to and caused by %+v", hits, target, casts[0].Cause)
	}
	if ended := endedShots(t, w, "bob"); len(ended) != 1 || hits[0].Until != ended[0].Until {
		t.Fatalf("damage lands at %v of the tick, want when the shot ended %+v", hits[0].Until, ended)
	}
}

// With no claim, a Body's shot passes Bodies and Echoes: it flies through bob's Echo.
func TestShotWithoutClaimPassesEnemies(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, 10) // off his Echo
	k, _ := fireAtEcho(t, w, alice)
	flyFor(w, k+3)

	got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	if n := len(shots(t, w, "alice")); got.HP != got.MaxHP || n != 1 {
		t.Fatalf("bob's Echo HP = %v with %d shots left, want %v and the shot flying on", got.HP, n, got.MaxHP)
	}
}

// An enemy Body on the path in front of the claimed Echo, that tick and where alice drew it, blocks the
// claim: bob stood half over his Echo, toward her, and walked off after her view. The shot flies on.
func TestEnemyBodyBlocksShot(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, -1, 0, 2)

	k, h := fireAtEcho(t, w, alice)
	for i := 1; i <= k; i++ {
		if i > k-7 {
			bob.move(0, 1)
		}
		if i == k {
			alice.claim(h, 7.5) // from before he left
		}
		w.Tick()
	}
	flyFor(w, 2)

	got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	if n := len(shots(t, w, "alice")); got.HP != got.MaxHP || n != 1 {
		t.Fatalf("bob's Echo HP = %v behind his Body with %d shots left, want %v and the shot flying on",
			got.HP, n, got.MaxHP)
	}
}

// A Body standing on its own Echo touches the shot at the same moment, and wins the tie: a claim on the
// Echo is rejected.
func TestBodyOnItsEchoShieldsIt(t *testing.T) {
	w, alice, _ := duel(t)
	k, h := fireAtEcho(t, w, alice)
	claimAt(w, alice, h, drawLag, k)
	flyFor(w, 2)

	got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	if n := len(shots(t, w, "alice")); got.HP != got.MaxHP || n != 1 {
		t.Fatalf("bob's Echo HP = %v under his Body with %d shots left, want %v and the shot flying on",
			got.HP, n, got.MaxHP)
	}
}

// A claim is checked where alice drew the target: bob walked off the shot's path after the view she
// claimed, and the shot still stops on him.
func TestClaimHitsWhereTheViewHadIt(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, -1, 10) // off his Echo
	from, to := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	alice.fire(to.X-from.X, to.Y-from.Y)
	w.Tick()
	h := claimOn(t, w, alice, to.EntityID)
	k := firstTouch(t, w, alice, math.Hypot(to.X-from.X, to.Y-from.Y), to.Radius) // where he stood
	// Her view at step k is from before he leaves, 7 ticks before it.
	const back = 7.5
	for i := 1; i <= k; i++ {
		if n := len(shots(t, w, "alice")); n != 1 {
			t.Fatalf("%d shots left after %d steps, want the shot until step %d", n, i, k)
		}
		if i > k-7 {
			bob.move(0, 1)
		}
		if i == k {
			alice.claim(h, back)
		}
		w.Tick()
	}
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("%d shots left, want the shot stopped where she saw bob", n)
	}
	if b := yourBody(t, bodiesFor(t, w, "bob")); b.Y-to.Y < 1.5 {
		t.Fatalf("bob moved %.2f, want him off the path now", b.Y-to.Y)
	}
}

// A claim is checked where alice's screen drew bob, partway between two of his rows: he dashes up
// across her shot's path, a unit a tick. Drawn 0.3 of the way from the row on the path, he is hit there;
// drawn 0.8 of the way, the shot passes under him and flies on.
func TestClaimDrawsTargetBetweenRows(t *testing.T) {
	for _, c := range []struct {
		part float64
		hits bool
	}{{0.3, true}, {0.8, false}} {
		w, alice, bob := duel(t)
		moveFor(w, bob, -1, 0, 8) // off his Echo, toward her
		moveFor(w, bob, 0, -1, 4) // a unit below her shot's path
		from, to := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
		alice.fire(1, 0)
		w.Tick()
		h := claimOn(t, w, alice, to.EntityID)
		// Drawn 0.3 up, his circle first touches the path in step k; the dash cast at step 5 moves him
		// from step 6 on, so her view at step k is between his rows at steps 6 and 7.
		_, shot := primary(t)
		reach := to.Radius + shot.Radius
		k := int((to.X - from.X - math.Sqrt(reach*reach-0.3*0.3)) / (shot.Speed * physics.SecondsPerTick))
		dashSlot, _ := dash(t)
		for i := 1; i <= k; i++ {
			if i == 5 {
				bob.cast(dashSlot, 0, 1)
			}
			if i == k {
				alice.claim(h, float64(k-6)-c.part)
			}
			w.Tick()
		}
		if gone := len(shots(t, w, "alice")) == 0; gone != c.hits {
			t.Errorf("drawn %v of the way: the shot stopped on bob: %v, want %v", c.part, gone, c.hits)
		}
	}
}

// A claim the server can't check is rejected, and the shot flies on: a view past rewind_seconds or not
// saved yet, a target that isn't there, or a key that names no shot of hers.
func TestBadClaimsAreRejected(t *testing.T) {
	keep := float64(tuningOf[projectile.ProjectileTuning](t).RewindTicks())
	for _, c := range []struct {
		name string
		back float64
		bad  func(*playercomp.HitClaim)
	}{
		{"too old", keep + 0.5, nil},
		{"not saved yet", 0.5, nil},
		{"no such target", drawLag, func(h *playercomp.HitClaim) { h.Target = 4294967295 }},
		{"no such shot", drawLag, func(h *playercomp.HitClaim) { h.Launch = 1 }},
		{"another ability's", drawLag, func(h *playercomp.HitClaim) { h.Ability = kit(t)[1] }},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, alice, bob := duel(t)
			moveFor(w, bob, 0, 1, 10) // off his Echo
			k, h := fireAtEcho(t, w, alice)
			if c.bad != nil {
				c.bad(&h)
			}
			claimAt(w, alice, h, c.back, k)
			flyFor(w, 2)
			got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
			if n := len(shots(t, w, "alice")); got.HP != got.MaxHP || n != 1 {
				t.Fatalf("bob's Echo HP = %v with %d shots left, want %v and the shot flying on", got.HP, n, got.MaxHP)
			}
		})
	}
}

// A wall on the path in front of the claimed Echo, that tick, stops the shot first: the claim is
// rejected.
func TestWallBeforeClaimedEchoStopsShot(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, 10) // off his Echo
	target := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	a := yourBody(t, bodiesFor(t, w, "alice"))
	// alice walks to 1.25 from the Echo, so her shot's first step reaches it, and a thin wall goes between.
	speed := bodiesFor(t, w, "alice").You.MoveSpeed * physics.SecondsPerTick
	moveFor(w, alice, 1, 0, int(math.Round((target.X-1.25-a.X)/speed)))
	a = yourBody(t, bodiesFor(t, w, "alice"))
	addWall(w, physics2d.Vec2{X: (a.X + target.X) / 2, Y: target.Y}, physics2d.Vec2{X: 0.05, Y: 2})
	w.Tick() // physics picks the wall up

	alice.fireClaiming(t, 1, 0, target.EntityID, drawLag)
	w.Tick()
	got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	if n := len(shots(t, w, "alice")); got.HP != got.MaxHP || n != 0 {
		t.Fatalf("bob's Echo HP = %v behind a wall with %d shots left, want %v and the shot stopped",
			got.HP, n, got.MaxHP)
	}
}

// A shot no frame cast, like an Echo's, meets Bodies and Echoes where they are now, with no claim.
func TestEchoShotHitsLive(t *testing.T) {
	w, _, bob, fx := fxDuel(t)
	moveFor(w, bob, 0, 1, 10) // off his Echo
	mine := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice")))
	target := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	cause := abilitycomp.Cause{Caster: cardinal.EntityID(mine.EntityID), Ability: kit(t)[0]}
	emit(fx, abilitycomp.AbilityCast{Cause: cause, AimX: target.X - mine.X, AimY: target.Y - mine.Y})
	_, shot := primary(t)
	ended := flyUntilEnded(t, w, "alice",
		int(math.Ceil(math.Hypot(target.X-mine.X, target.Y-mine.Y)/(shot.Speed*physics.SecondsPerTick)))+1)

	got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	if n := len(shots(t, w, "alice")); got.HP != got.MaxHP-shot.Damage || n != 0 {
		t.Fatalf("bob's Echo HP = %v with %d shots left, want %v and the shot gone", got.HP, n, got.MaxHP-shot.Damage)
	}
	if len(ended) != 1 || !touching(ended[0], target.X, target.Y, target.Radius) {
		t.Fatalf("shots ended at %+v, want one touching bob's Echo at (%v, %v)", ended, target.X, target.Y)
	}
}

// A claim on an ally is rejected, since shots pass allies: alice's shot flies through carol.
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
		History   projectilecomp.TargetHistory
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

	flyFor(w, 4) // carol's rows reach back to alice's view

	// The shot starts inside alice's own Body and Echo, then crosses carol.
	alice.fire(1, 0)
	w.Tick()
	claimAt(w, alice, claimOn(t, w, alice, uint32(carol.ID())), drawLag, firstTouch(t, w, alice, 2, 0.5))
	flyFor(w, 3)

	s := shots(t, w, "alice")
	if len(s) != 1 || s[0].X <= from.X+2.5 {
		t.Fatalf("shots = %+v, want one past carol (x > %.2f)", s, from.X+2.5)
	}
}

// A shot ends at its range in every direction, though a rounded speed can leave its last step short.
// Each is sent once more, at its range.
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
	ended := flyUntilEnded(t, w, "alice", 1)
	if got := len(shots(t, w, "alice")); got != 0 || len(ended) != n {
		t.Fatalf("%d shots still flying and %d ended after %d steps, want all %d ended", got, len(ended), steps, n)
	}
	for _, s := range ended {
		if d := math.Hypot(s.X-100, s.Y-100); math.Abs(d-shot.Range) > 1e-9 || !(s.Until > 0 && s.Until <= 1) {
			t.Fatalf("shot %d ended %v from its start, Until %v, want its range %v", s.EntityID, d, s.Until, shot.Range)
		}
	}
}

func TestWallStopsShot(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, 10) // off his Echo, so only the wall can stop the shot

	// A wall halfway between them.
	a, b := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	wallX := (a.X + b.X) / 2
	addWall(w, physics2d.Vec2{X: wallX, Y: 0}, physics2d.Vec2{X: 0.5, Y: 5})
	w.Tick() // physics picks the wall up

	k, _ := fireAtEcho(t, w, alice)
	ended := flyUntilEnded(t, w, "alice", k+2)
	// Sent once more, touching the wall's face, half its width from its center.
	if len(ended) != 1 || math.Abs(math.Abs(ended[0].X-wallX)-(0.5+ended[0].Radius)) > 2*box2d.LinearSlop {
		t.Fatalf("shots ended at %+v, want one touching the wall's face", ended)
	}

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

	alice.fireClaiming(t, 0, 1, target.EntityID, drawLag) // away from the Echo's center
	w.Tick()
	_, shot := primary(t)
	if got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))); got.HP != got.MaxHP-shot.Damage {
		t.Fatalf("bob's Echo HP = %v after a point-blank shot, want %v", got.HP, got.MaxHP-shot.Damage)
	}
}

// A shot no frame cast meets an Echo where its replay puts it this tick, not a step behind: the replay
// runs before the physics step, so Box2D holds the new pose when shots query it.
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

// Each Body and Echo keeps its last rewind_seconds of rows, one per tick, and its newest row is what this
// tick's syncs show of it: a client that claims a hit sees the targets the server checks it against.
func TestTargetHistoryIsWhatSyncsShow(t *testing.T) {
	w, _, bob, fx := fxDuel(t)
	fx.effect(yourBody(t, bodiesFor(t, w, "bob")), effectcomp.EffectIntent{Kind: "stun", Ticks: 3})
	keep := int(tuningOf[projectile.ProjectileTuning](t).RewindTicks())
	stunned := false
	newest := func(id uint32) []flight.Row {
		return slices.Collect(w.Entity(cardinal.EntityID(id)).Get[projectilecomp.TargetHistory]().Rows.Values())
	}
	check := func(what string, rows []flight.Row, want flight.Row, full bool) {
		t.Helper()
		if len(rows) == 0 || len(rows) > keep || full && len(rows) != keep || rows[len(rows)-1] != want {
			t.Fatalf("%s: %d rows ending %v, want %d ending %+v", what, len(rows), rows[max(len(rows)-1, 0):], keep, want)
		}
		for i, r := range rows {
			if r.Tick != want.Tick-uint64(len(rows)-1-i) {
				t.Fatalf("%s: row %d is tick %d, want one row per tick up to %d", what, i, r.Tick, want.Tick)
			}
		}
	}
	for i := range keep + 4 {
		bob.move(0, 1) // stunned at first, then he walks
		w.Tick()
		bodies, echoes := bodiesFor(t, w, "alice"), sentTo[echo.EchoSync](t, w, "alice")
		for b := range bodies.Bodies.Values() {
			stunned = stunned || b.Tags&interact.Stunned != 0
			check(fmt.Sprintf("Body %d", b.EntityID), newest(b.EntityID),
				flight.Row{Tick: bodies.Tick, Center: box2d.Vec2{X: b.X, Y: b.Y}, Radius: b.Radius, Tags: b.Tags, Until: 1},
				i >= keep)
		}
		for e := range echoes.Echoes.Values() {
			check(fmt.Sprintf("Echo %d", e.EntityID), newest(e.EntityID),
				flight.Row{Tick: echoes.Tick, Center: box2d.Vec2{X: e.X, Y: e.Y}, Radius: e.Radius, Tags: e.Tags, Until: e.Until},
				i >= keep)
			if owner := w.Entity(cardinal.EntityID(e.EntityID)).Get[team.Owner]().Body; uint32(owner) != e.Owner {
				t.Fatalf("Echo %d says its owner is %d, not %d", e.EntityID, e.Owner, owner)
			}
		}
	}
	if !stunned {
		t.Fatal("no sync showed bob stunned")
	}
}

// shootEcho has alice fire kit slot 0 at bob's Echo, claim it hit it on the step it first touches it,
// and tick until the shot is gone. It returns his Echo's HP then.
func shootEcho(t *testing.T, w *cardinal.TestWorld, alice *client) float64 {
	t.Helper()
	from, bob := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	to := echoOf(t, w, "alice", bob)
	alice.fire(to.X-from.X, to.Y-from.Y)
	w.Tick()
	k := firstTouch(t, w, alice, math.Hypot(to.X-from.X, to.Y-from.Y), to.Radius)
	claimAt(w, alice, claimOn(t, w, alice, to.EntityID), drawLag, k)
	for range 100 {
		if len(shots(t, w, "alice")) == 0 {
			return echoOf(t, w, "alice", bob).HP
		}
		w.Tick()
	}
	t.Fatal("the shot is still flying after 100 ticks")
	return 0
}

// A shot type's own rules come before the defaults: a shot that passes Bodies hits the Echo behind an
// enemy Body, half over it (test content).
func TestShotTypeRulesComeFirst(t *testing.T) {
	w := cardinal.NewTestWorld(t, withContent(t, map[string]string{
		"ability/tuning.json": `{"kits": {"starter": ["lance"]},
			"abilities": {"lance": {"aimed": true, "hook": "launch", "params": {"shot": "lance"}}}}`,
		"projectile/tuning.json": `{"shots": {"lance": {"speed": 15, "range": 20, "radius": 0.15, "damage": 10,
			"meets": [{"kinds": ["body"], "do": "pass"}]}}}`,
	}))
	alice, bob := claim(t, w)
	moveFor(w, bob, -1, 0, 2) // half over his Echo, toward alice
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
	moveFor(w, bob, -1, 0, 2) // half over his Echo, toward alice
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
// kit casts nothing. A hit claim's check finds the targets in front within max_target_speed, so nothing
// moves faster.
func TestContentIDsLineUp(t *testing.T) {
	abilities := tuningOf[ability.AbilityTuning](t)
	projectiles := tuningOf[projectile.ProjectileTuning](t)
	shots, kinds := projectiles.Shots, tuningOf[effect.EffectTuning](t).Kinds
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
	for id, c := range champions {
		if c.MoveSpeed > projectiles.MaxTargetSpeed {
			t.Errorf("champion %q moves %v units/s, past max_target_speed %v", id, c.MoveSpeed, projectiles.MaxTargetSpeed)
		}
	}
	for id, def := range abilities.Abilities {
		if def.Hook != "dash" {
			continue
		}
		var d dashParams
		if err := json.Unmarshal(def.Params, &d); err != nil {
			t.Fatal(err)
		}
		if speed := d.Distance / (float64(d.Ticks()) * physics.SecondsPerTick); speed > projectiles.MaxTargetSpeed {
			t.Errorf("ability %q dashes %v units/s, past max_target_speed %v", id, speed, projectiles.MaxTargetSpeed)
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
	if len(hits) != 1 || hits[0].Target != touching || hits[0].Until != 1 {
		t.Fatalf("damage %+v, want one over the tick, on the area %d that touches her", hits, touching)
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

	// Emitted out of time order: the second hit to land takes it to 0, though by size it'd be the third.
	emit(fx, combatcomp.DamageIntent{Target: target, Amount: 30, Until: 0.75})
	emit(fx, combatcomp.DamageIntent{Target: target, Amount: e.MaxHP - 20, Until: 0.25})
	emit(fx, combatcomp.DamageIntent{Target: target, Amount: 20, Until: 0.5})
	mine := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice")))
	emit(fx, combatcomp.HealIntent{Target: cardinal.EntityID(mine.EntityID), Amount: -mine.MaxHP}) // a bad heal
	w.Tick()
	// Sent once more, at HP 0, alive until the hit that took it there landed: no hit took hers.
	want := map[uint32]float64{e.EntityID: 0.5, mine.EntityID: 1}
	for other := range sentTo[echo.EchoSync](t, w, "alice").Echoes.Values() {
		if until, ok := want[other.EntityID]; ok {
			if other.HP != 0 || other.Until != until {
				t.Fatalf("dead Echo %d sent at %v HP until %v, want 0 HP until %v", other.EntityID, other.HP, other.Until, until)
			}
			delete(want, other.EntityID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("dead Echoes %v weren't sent once more", want)
	}
	w.Tick()
	for other := range sentTo[echo.EchoSync](t, w, "alice").Echoes.Values() {
		if other.EntityID == e.EntityID || other.EntityID == mine.EntityID {
			t.Fatalf("Echo %d is still here a tick after it died", other.EntityID)
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

// predictionStep is one tick of TestPredictionMatchesServer's run: the body_sync and echo_sync before
// it, the frame the server applied with the predictor's claims on it, and what the predictor gave after
// it, as predictor.wasm returns it.
type predictionStep struct {
	Sync   player.BodySync
	Echoes echo.EchoSync
	Frame  playercomp.InputFrame
	X, Y   float64          // your Body
	Shots  []shotPrediction // your shots, in the predictor's order
	Ended  []endedPrediction
	Kit    kitPrediction
}

// shotPrediction is one of your shots, as predictor.wasm's shots() returns it.
type shotPrediction struct {
	Type                       string
	X, Y, PrevX, PrevY, Radius float64
}

// endedPrediction is one of your shots that ended in a step, as predictor.wasm's step() returns it.
type endedPrediction struct {
	Type                              string
	X, Y, PrevX, PrevY, Radius, Until float64
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

// overWire is v after a MessagePack round trip, as a client gets it.
func overWire[T any](t *testing.T, v T) T {
	t.Helper()
	raw, err := msgpack.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var got T
	if err := msgpack.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

// predictFrom is the predictor's start from a body_sync, as client/src/game/player/predictor.ts builds
// it from s as a client gets it.
func predictFrom(t *testing.T, s player.BodySync) predict.State {
	t.Helper()
	got := overWire(t, s)
	b, you := yourBody(t, got), got.You
	state := predict.State{
		Tick: got.Tick, Body: you.Body, Team: b.Team, Center: box2d.Vec2{X: b.X, Y: b.Y}, Radius: b.Radius,
		MoveSpeed: you.MoveSpeed, Champion: you.Champion, Cooldowns: slices.Collect(you.Cooldowns.Values()),
		Effects: slices.Collect(you.Effects.Values()), EffectsNext: you.EffectsNext,
	}
	for shot := range you.Shots.Values() {
		key := predict.Key{Seq: shot.Seq, Ability: shot.Ability, Launch: shot.Launch}
		state.Shots = append(state.Shots, predict.Shot{Type: shot.Type, Key: key, Flight: flight.Flight{
			Pos:      box2d.Vec2{X: shot.X, Y: shot.Y},
			Velocity: box2d.Vec2{X: shot.VelX, Y: shot.VelY}, Radius: shot.Radius, Remaining: shot.Remaining,
		}})
	}
	return state
}

// see gives p the Bodies and Echoes a client's body_sync and echo_sync show, as predictor.ts does.
func see(t *testing.T, p *predict.Predictor, bodies player.BodySync, echoes echo.EchoSync) {
	t.Helper()
	var sights []predict.Sight
	for b := range overWire(t, bodies).Bodies.Values() {
		sights = append(sights, predict.Sight{ID: b.EntityID, Side: interact.Side{Owner: b.EntityID, Team: b.Team},
			Center: box2d.Vec2{X: b.X, Y: b.Y}, Radius: b.Radius, Tags: b.Tags, Until: 1})
	}
	p.See(bodies.Tick, interact.Body, sights)
	sights = nil
	for e := range overWire(t, echoes).Echoes.Values() {
		sights = append(sights, predict.Sight{ID: e.EntityID, Side: interact.Side{Owner: e.Owner, Team: e.Team},
			Center: box2d.Vec2{X: e.X, Y: e.Y}, Radius: e.Radius, Tags: e.Tags, Until: e.Until})
	}
	p.See(echoes.Tick, interact.Echo, sights)
}

// predictFrame is f as the predictor takes it.
func predictFrame(f playercomp.InputFrame) predict.Frame {
	out := predict.Frame{
		Seq: f.Seq, MoveX: f.MoveX, MoveY: f.MoveY, AimX: f.AimX, AimY: f.AimY, Buttons: f.Buttons, ViewTick: f.ViewTick,
	}
	for c := range f.Claims.Values() {
		key := predict.Key{Seq: c.Seq, Ability: c.Ability, Launch: c.Launch}
		out.Claims = append(out.Claims, predict.Claim{Key: key, Target: c.Target})
	}
	return out
}

// claimsOf is claims as a frame carries them.
func claimsOf(claims []predict.Claim) cardinal.Slice[playercomp.HitClaim] {
	var out []playercomp.HitClaim
	for _, c := range claims {
		out = append(out, playercomp.HitClaim{Seq: c.Seq, Ability: c.Ability, Launch: c.Launch, Target: c.Target})
	}
	return cardinal.SliceOf(out...)
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
			out[i] = predict.Shot{Type: s.Type, Key: s.Key, Flight: s.Flight}
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
func record(p *predict.Predictor, sync player.BodySync, echoes echo.EchoSync, f playercomp.InputFrame,
	ended []predict.Ended,
) predictionStep {
	s := p.State()
	step := predictionStep{
		Sync: sync, Echoes: echoes, Frame: f, X: s.Center.X, Y: s.Center.Y, Shots: []shotPrediction{},
		Ended: []endedPrediction{}, Kit: kitPrediction{Tick: float64(s.Tick), Slots: []kitSlot{}},
	}
	for _, shot := range s.Shots {
		step.Shots = append(step.Shots, shotPrediction{
			Type: shot.Type, X: shot.Pos.X, Y: shot.Pos.Y, PrevX: shot.Prev.X, PrevY: shot.Prev.Y, Radius: shot.Radius,
		})
	}
	for _, e := range ended {
		step.Ended = append(step.Ended, endedPrediction{
			Type: e.Type, X: e.End.At.X, Y: e.End.At.Y, PrevX: e.Pos.X, PrevY: e.Pos.Y, Radius: e.Radius, Until: e.End.Until,
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
// and a shot flying out its range. Her second bolt meets bob, walking across it, where her screen drew
// him: the predictor's claim is the one the server accepts, the same tick, and a replay of the frame
// ends the shot too. Effects others put on her surprise it, and come with the next state.
// testdata/prediction.msgpack keeps the run, so the client checks that its wasm build lands there too
// (client/src/game/player/predictor.test.ts).
func TestPredictionMatchesServer(t *testing.T) {
	w, alice, bob, fx := fxDuel(t)
	rules, err := predict.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	p := predict.New(rules)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	from := yourBody(t, bodiesFor(t, w, "bob")).EntityID
	dashSlot, _ := dash(t)
	boltCast, bolt := primary(t)
	second := int(boltCast.CooldownTicks()) // the step her bolt is ready again
	step := bolt.Speed * physics.SecondsPerTick
	if steps := int(bolt.Range / step); steps >= second {
		t.Fatalf("a bolt flies %d ticks, past its cooldown", steps)
	}
	// What bob puts on her, by step. Each applies from the tick after it.
	surprises := map[int]effectcomp.EffectIntent{
		2: {Kind: "slow", Ticks: 9, Value: -4000, Source: from},
		6: {Kind: "stun", Ticks: 3, Source: from}, // ends her dash
	}
	moves := [][2]float64{{1, 0}, {0.3, -0.9}, {-1, 1}, {0, 0}, {math.NaN(), 1}, {-0.6, -0.8}, {3, 4}, {1e-12, 0}}
	// She holds fire for six ticks, aiming up and left, away from bob: it casts once, then cools down.
	// She also dashes on the fourth. Once her bolt is ready, she holds fire again, leading bob.
	var lead box2d.Vec2
	frameAt := func(i int) playercomp.InputFrame {
		m := moves[i%len(moves)]
		f := playercomp.InputFrame{MoveX: m[0], MoveY: m[1], AimX: -0.6, AimY: 0.8}
		switch {
		case i == 3:
			f.Buttons = 1 | 1<<dashSlot
		case i < 6:
			f.Buttons = 1
		case i >= second && i < second+6:
			f.Buttons, f.AimX, f.AimY = 1, lead.X, lead.Y
		}
		return f
	}
	const lag = 2.3 // ticks her screen draws others behind the server: a fractional view
	walk := bodiesFor(t, w, "bob").You.MoveSpeed * physics.SecondsPerTick

	// One frame from each tick's state, as when the server acks every frame, for two cooldowns. Her
	// client sees each tick's syncs, steps the frame from the newest state, and sends it with its claims.
	var run []predictionStep
	var claimed []predict.Claim
	var ends []predict.Ended
	for i := range 2 * second {
		e, surprised := surprises[i]
		if surprised {
			fx.effect(a, e)
		}
		if i >= second-6 {
			bob.move(0, 1) // up across her second bolt's path
		}
		before, echoes := bodiesFor(t, w, "alice"), sentTo[echo.EchoSync](t, w, "alice")
		see(t, p, before, echoes)
		p.Reset(predictFrom(t, before))
		f := frameAt(i)
		if i == second {
			// Aim where her screen will draw bob when the bolt gets there: at step k, his rows from k-3
			// and k-2, 0.7 of the way.
			m, b := moves[i%len(moves)], yourBody(t, bodiesFor(t, w, "bob"))
			launch := p.Peek(m[0], m[1])
			k := 0.0
			for range 20 {
				lead = box2d.Vec2{X: b.X - launch.X, Y: b.Y + walk*(k+1-lag) - launch.Y}
				k = math.Hypot(lead.X, lead.Y)/step - 0.5
			}
			f = frameAt(i)
		}
		f.Seq, f.ViewTick = alice.seq+1, float64(w.TickHeight())-lag
		_, claims, ended := p.Step(predictFrame(f))
		f.Claims = claimsOf(claims)
		claimed = append(claimed, claims...)
		ends = append(ends, ended...)
		alice.send(f)
		w.Tick()
		after := bodiesFor(t, w, "alice")
		requirePredicted(t, fmt.Sprintf("step %d", i), p, after, !surprised)
		run = append(run, record(p, before, echoes, f, ended))
		p.Reset(predictFrom(t, before))
		p.Replay(predictFrame(f))
		requirePredicted(t, fmt.Sprintf("step %d replayed", i), p, after, !surprised)
	}
	if len(run[0].Shots) != 1 || len(run[second-1].Shots) != 0 {
		t.Fatalf("predicted %d shots after her first frame and %d before her second bolt, want one, then none",
			len(run[0].Shots), len(run[second-1].Shots))
	}
	if bobBody := yourBody(t, bodiesFor(t, w, "bob")).EntityID; len(claimed) != 1 || claimed[0].Target != bobBody {
		t.Fatalf("claims %+v, want one: her second bolt on bob's Body %d", claimed, bobBody)
	}
	// Her first bolt is drawn out to its range and her second up to bob, each at its speed in its last tick.
	launch := box2d.Vec2{X: run[0].Shots[0].PrevX, Y: run[0].Shots[0].PrevY}
	if len(ends) != 2 || math.Abs(math.Hypot(ends[0].End.At.X-launch.X, ends[0].End.At.Y-launch.Y)-bolt.Range) > 1e-9 ||
		ends[1].Key != claimed[0].Key || ends[1].End.Hit.Target != claimed[0].Target {
		t.Fatalf("ended %+v, want her first bolt at its range from %v, then her second on bob", ends, launch)
	}
	for _, e := range ends {
		if flew := math.Hypot(e.End.At.X-e.Pos.X, e.End.At.Y-e.Pos.Y); !(e.End.Until > 0 && e.End.Until <= 1) ||
			math.Abs(flew-e.End.Until*step) > 1e-9 {
			t.Fatalf("%+v flew %v in its last tick, want Until (0 to 1) of a tick's %v", e, flew, step)
		}
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
		f := alice.send(playercomp.InputFrame{MoveX: m[0], MoveY: m[1], AimX: aim[0], AimY: aim[1], Buttons: 1})
		w.Tick()
		p.Step(predictFrame(f))
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
	p.Forget()
	for i, s := range recorded {
		see(t, p, s.Sync, s.Echoes)
		p.Reset(predictFrom(t, s.Sync))
		_, claims, ended := p.Step(predictFrame(s.Frame))
		got := record(p, s.Sync, s.Echoes, s.Frame, ended)
		if got.X != s.X || got.Y != s.Y || !slices.Equal(got.Shots, s.Shots) || got.Kit.Tick != s.Kit.Tick ||
			!slices.Equal(got.Kit.Slots, s.Kit.Slots) || !slices.Equal(claims, predictFrame(s.Frame).Claims) ||
			!slices.Equal(got.Ended, s.Ended) {
			t.Fatalf("%s step %d: predicted (%v, %v), shots %+v, ended %+v, kit %+v, claims %+v; recorded (%v, %v), "+
				"%+v, %+v, %+v, %+v. If the sim changed on purpose, run "+
				"`go test ./shards/game -run TestPredictionMatchesServer -update`", path, i, got.X, got.Y, got.Shots,
				got.Ended, got.Kit, claims, s.X, s.Y, s.Shots, s.Ended, s.Kit, predictFrame(s.Frame).Claims)
		}
	}
}

// The predictor places a Body or Echo only from both syncs of both kinds around a frame's view, and only
// while the newest sync still shows it. Otherwise her shot passes where her screen drew it. A hit ends
// her shot touching it there, its last tick at its speed.
func TestPredictorPlacesOnlyWhatSyncsShow(t *testing.T) {
	rules, err := predict.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	_, bolt := primary(t)
	step := bolt.Speed * physics.SecondsPerTick
	// The enemy stands on her shot's path, or farther on it, or a tick later 2 units up off it, or dies
	// halfway through that tick there.
	enemy := predict.Sight{ID: 7, Side: interact.Side{Owner: 7, Team: 1}, Center: box2d.Vec2{X: 1}, Radius: 0.5, Until: 1}
	far, up := enemy, enemy
	far.Center.X = 2
	up.Center.Y = 2
	dying, dyingUp := enemy, up
	dying.Until, dyingUp.Until = 0.5, 0.5
	both := func(p *predict.Predictor, tick uint64, s predict.Sight) {
		p.See(tick, interact.Body, []predict.Sight{s})
		p.See(tick, interact.Echo, nil)
	}
	at := func(x, y float64) *box2d.Vec2 { return &box2d.Vec2{X: x, Y: y} }
	for _, c := range []struct {
		name  string
		see   func(p *predict.Predictor)
		view  float64
		drawn *box2d.Vec2 // where her screen drew the enemy, if her shot hits it
	}{
		{"seen", func(p *predict.Predictor) { both(p, 10, enemy); both(p, 11, enemy) }, 10.5, at(1, 0)},
		{"seen a tick's flight away", func(p *predict.Predictor) { both(p, 10, far); both(p, 11, far) }, 10.5, at(2, 0)},
		{"no echo_syncs", func(p *predict.Predictor) {
			p.See(10, interact.Body, []predict.Sight{enemy})
			p.See(11, interact.Body, []predict.Sight{enemy})
		}, 10.5, nil},
		{"gone from the newest", func(p *predict.Predictor) {
			both(p, 10, enemy)
			both(p, 11, enemy)
			p.See(12, interact.Body, nil)
		}, 10.5, nil},
		{"drawn a quarter of the way up", func(p *predict.Predictor) { both(p, 10, enemy); both(p, 11, up) }, 10.25, at(1, 0.5)},
		{"drawn most of the way up", func(p *predict.Predictor) { both(p, 10, enemy); both(p, 11, up) }, 10.9, nil},
		{"seen before it died", func(p *predict.Predictor) { both(p, 10, enemy); both(p, 11, dying) }, 10.25, at(1, 0)},
		{"seen after it died", func(p *predict.Predictor) { both(p, 10, enemy); both(p, 11, dying) }, 10.75, nil},
		{"drawn up at its own pace as it died", func(p *predict.Predictor) { both(p, 10, enemy); both(p, 11, dyingUp) },
			10.1, at(1, 0.4)},
	} {
		p := predict.New(rules)
		c.see(p)
		p.Reset(predict.State{Tick: 12, Body: 1, Radius: 0.5, MoveSpeed: 5, Champion: "starter"})
		var claims []predict.Claim
		var ended []predict.Ended
		for i, buttons := range []uint32{1, 0} { // she casts, then holds
			_, stepClaims, stepEnded := p.Step(predict.Frame{Seq: uint32(i + 1), AimX: 1, Buttons: buttons, ViewTick: c.view})
			claims, ended = append(claims, stepClaims...), append(ended, stepEnded...)
		}
		hits := c.drawn != nil
		if hit := len(claims) == 1 && claims[0].Target == enemy.ID; hit != hits || len(claims) > 1 {
			t.Errorf("%s: claims %+v, want a hit on %d: %v", c.name, claims, enemy.ID, hits)
		}
		if !hits {
			continue
		}
		if len(ended) != 1 {
			t.Errorf("%s: ended %+v, want her shot", c.name, ended)
			continue
		}
		// Box2D stops a cast a slop short of touching.
		e, to := ended[0], ended[0].End.At
		if math.Abs(math.Hypot(to.X-c.drawn.X, to.Y-c.drawn.Y)-(bolt.Radius+enemy.Radius)) > 2*box2d.LinearSlop ||
			math.Abs(math.Hypot(to.X-e.Pos.X, to.Y-e.Pos.Y)-e.End.Until*step) > 1e-9 {
			t.Errorf("%s: ended %+v, want her shot touching the enemy at %v, at its speed", c.name, e, *c.drawn)
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
			ViewTick: 41.375, Claims: cardinal.SliceOf(
				playercomp.HitClaim{Seq: 1, Ability: "bolt", Launch: 1, Target: 4294967295},
				playercomp.HitClaim{Seq: 2, Ability: "nova", Target: 3},
			),
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
					Type: "bolt", Ability: "bolt", Seq: 4294967295, Launch: 1, X: -4.25, Y: 1.5, VelX: 9, VelY: -12,
					Radius: 0.15, Remaining: 17.75,
				}),
			},
			Bodies: cardinal.SliceOf(
				player.BodyState{EntityID: 1, PlayerID: 1, Nickname: "Player One", Team: 0, X: 1.5, Y: -2, Radius: 0.5},
				player.BodyState{EntityID: 2, PlayerID: 2, Nickname: "Échø ✦", Team: 1, X: -0.25, Y: 3.75, Radius: 0.75,
					Tags: interact.Stunned | interact.CC},
			),
		}},
		{"echo_sync", echo.EchoSync{Tick: 7, Echoes: cardinal.SliceOf(
			echo.EchoState{EntityID: 3, PlayerID: 1, Owner: 1, Team: 0, X: -4.5, Y: 0.5, Radius: 0.5,
				Tags: interact.Unstoppable, HP: 62.5, MaxHP: 100, Until: 1},
			echo.EchoState{EntityID: 4, PlayerID: 2, Owner: 2, Team: 1, X: 3, Y: -1.25, Radius: 0.5, MaxHP: 100, Until: 0.625},
		)}},
		{"ability_sync", ability.AbilitySync{Tick: 7, Abilities: cardinal.SliceOf(
			ability.AbilityState{Body: 1, Slot: 0, Ability: "bolt", ReadyTick: 18446744073709551615},
		)}},
		{"projectile_sync", projectile.ProjectileSync{Tick: 8, Projectiles: cardinal.SliceOf(
			projectile.ProjectileState{EntityID: 9, Type: "bolt", Caster: 4294967295, Team: 1, X: 0.125, Y: -7.5, Radius: 0.15,
				Until: 1},
			projectile.ProjectileState{EntityID: 10, Type: "bolt", Caster: 3, Team: 0, X: 2.5, Y: 1, Radius: 0.15,
				Until: 0.375, Ended: true},
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
		if got.Session != in.Session || !reflect.DeepEqual(gotFrames, wantFrames) {
			t.Fatalf("the client's Input decodes as Session %d, Frames %+v; want %d, %+v. Update "+
				"client/src/game/player/input_command.ts, then run "+
				"`UPDATE_FIXTURES=1 bun x vitest run src/game/wire_fixtures.test.ts` in client",
				got.Session, gotFrames, in.Session, wantFrames)
		}
	})
}
