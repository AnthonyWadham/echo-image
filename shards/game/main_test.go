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
	"io/fs"
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
	"testing/fstest"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/vmihailenco/msgpack/v5"

	echoimage "echo_image"
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
	"echo_image/shards/game/internal/ref"
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
	files := patched(t, patches)
	return func(world *cardinal.World) { registerWith(world, files) }
}

// patched is the content withContent registers, which a predictor can load too.
func patched(t *testing.T, patches map[string]string) overlay {
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
	return files
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

// Open serves the same content as an fs.FS, for a predictor.
func (o overlay) Open(name string) (fs.File, error) {
	if raw, ok := o[name]; ok {
		return fstest.MapFS{name: {Data: raw}}.Open(name)
	}
	return content.FS.Open(name)
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

// client sends one input frame per call, numbering them the way the Unity client does. Its screen
// draws others back ticks behind the server, so each frame's view is that far before the tick it
// applies at.
type client struct {
	w       *cardinal.TestWorld
	id      string
	session uint32
	seq     uint32
	back    float64
}

// next is f numbered as c's next frame, with its view.
func (c *client) next(f playercomp.InputFrame) playercomp.InputFrame {
	f.Seq, f.ViewTick = c.seq+1, float64(c.w.TickHeight())-c.back
	return f
}

// send numbers f, gives it its view and sends it, and returns it as sent.
func (c *client) send(f playercomp.InputFrame) playercomp.InputFrame {
	f = c.next(f)
	c.seq = f.Seq
	c.w.Command(c.id, input(c.session, f))
	return f
}

// restart is c's client starting over: a new session, counting from 1 again.
func (c *client) restart() { c.session, c.seq = c.session+1, 0 }

func (c *client) move(x, y float64) { c.send(playercomp.InputFrame{MoveX: x, MoveY: y}) }

// cast holds the button for kit slot, aiming (aimX, aimY), and returns the frame's Seq.
func (c *client) cast(slot int, aimX, aimY float64) uint32 {
	return c.send(playercomp.InputFrame{AimX: aimX, AimY: aimY, Buttons: 1 << slot}).Seq
}

// fire casts kit slot 0.
func (c *client) fire(aimX, aimY float64) uint32 { return c.cast(0, aimX, aimY) }

// firstTouch is the step, counted from the one c's shot left on, whose path first touches a circle of
// radius r that lies dist from where it left, along its path. Its speed and size come from body_sync.
func firstTouch(t *testing.T, w *cardinal.TestWorld, c *client, dist, r float64) int {
	t.Helper()
	s := bodiesFor(t, w, c.id).You.Shots.At(0)
	return int((dist - r - s.Radius) / (math.Hypot(s.VelX, s.VelY) * physics.SecondsPerTick))
}

// drawLag is how far behind the server a client draws, in ticks: its interpolation delay and some ping,
// 0.125 s.
const drawLag = 0.125 * physics.ServerTickRateHz

// ticks is seconds in whole ticks at this build's rate, so a test states its times in seconds.
func ticks(seconds float64) int { return int(math.Round(seconds * physics.ServerTickRateHz)) }

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
	alice = &client{w: w, id: "alice", session: 1, back: drawLag}
	bob = &client{w: w, id: "bob", session: 1, back: drawLag}
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
// whose path first touches the Echo, short of the shot's range.
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
	dist := math.Hypot(to.X-from.X, to.Y-from.Y)
	if dist >= shot.Range-2*shot.Speed*physics.SecondsPerTick {
		t.Fatalf("bob's Echo is %.1f away, beyond the shot's range", dist)
	}
	return firstTouch(t, w, alice, dist, to.Radius)
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

// Everyone sees her shot leave her Body: its first tick flies from her spot, and each next from where
// the last one got to.
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
	a, s := yourBody(t, bodiesFor(t, w, "alice")), shots(t, w, "bob")[0]
	if s.Caster != body || s.Until != 1 || s.PrevX != a.X || s.PrevY != a.Y {
		t.Fatalf("the shot's Caster = %d, Until %v and start (%v, %v), want alice's Body %d, 1 and her spot (%v, %v)",
			s.Caster, s.Until, s.PrevX, s.PrevY, body, a.X, a.Y)
	}
	if mine := bodiesFor(t, w, "alice").You.Shots.At(0); mine.PrevX != a.X || mine.PrevY != a.Y {
		t.Fatalf("her own shot starts at (%v, %v), want her spot (%v, %v)", mine.PrevX, mine.PrevY, a.X, a.Y)
	}
	w.Tick()
	if next := shots(t, w, "bob")[0]; next.PrevX != s.X || next.PrevY != s.Y {
		t.Fatalf("the shot's second tick starts at (%v, %v), want where its first ended, (%v, %v)",
			next.PrevX, next.PrevY, s.X, s.Y)
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

// alice's shot meets bob's Echo where her screen drew it: the server stops the shot there and damages
// the Echo. Everyone is sent the shot once more, touching the Echo.
func TestShotDamagesEnemyEcho(t *testing.T) {
	w, alice, bob := duel(t)
	// bob steps off his Echo, which stays at his spawn while its trail warms up.
	moveFor(w, bob, 0, 1, ticks(0.5))

	flyFor(w, fireAtEcho(t, w, alice))

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
// HP 0, alive until the shot reached it, in its echo_sync and its target_sync alike. Then it's gone.
func TestKilledEchoLastsUntilTheShotReachesIt(t *testing.T) {
	_, shot := primary(t)
	w := cardinal.NewTestWorld(t, withContent(t, map[string]string{
		"echo/tuning.json": fmt.Sprintf(`{"max_hp": %v}`, shot.Damage),
	}))
	alice, bob := claim(t, w)
	moveFor(w, bob, 0, 1, ticks(0.5)) // off his Echo
	flyFor(w, fireAtEcho(t, w, alice))

	ended := endedShots(t, w, "bob")
	got := echoOf(t, w, "bob", yourBody(t, bodiesFor(t, w, "bob")))
	if len(ended) != 1 || got.HP != 0 || got.Until != ended[0].Until || ended[0].Until >= 1 ||
		!touching(ended[0], got.X, got.Y, got.Radius) {
		t.Fatalf("bob got his dying Echo %+v and shots ending %+v, want it at 0 HP until the shot touched it", got, ended)
	}
	if i := sentTo[projectile.TargetSync](t, w, "bob").Targets.IndexFunc(func(s projectile.TargetState) bool {
		return s.Serial == got.Serial && s.Until == got.Until
	}); i < 0 {
		t.Fatalf("bob's target_sync doesn't show his dying Echo until %v", got.Until)
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
	moveFor(w, bob, 0, 1, ticks(0.5)) // off his Echo
	k := fireAtEcho(t, w, alice)
	casts := w.Emitted[abilitycomp.AbilityCast]()
	if len(casts) != 1 {
		t.Fatalf("%d casts, want 1", len(casts))
	}
	flyFor(w, k)
	hits := w.Emitted[combatcomp.DamageIntent]()
	target := cardinal.EntityID(echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))).EntityID)
	if len(hits) != 1 || hits[0].Target != target || hits[0].Credit != casts[0].Caster || hits[0].Cause != casts[0].Cause {
		t.Fatalf("damage %+v, want one on bob's Echo %d, credited to and caused by %+v", hits, target, casts[0].Cause)
	}
	if ended := endedShots(t, w, "bob"); len(ended) != 1 || hits[0].Until != ended[0].Until {
		t.Fatalf("damage lands at %v of the tick, want when the shot ended %+v", hits[0].Until, ended)
	}
}

// An enemy Body on the path in front of bob's Echo where alice drew it stops the shot there: bob stood
// half over his Echo, toward her, and walked off after her view. The Echo takes no damage.
func TestEnemyBodyBlocksShot(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, -1, 0, ticks(0.1))
	drawn := yourBody(t, bodiesFor(t, w, "bob"))
	flyFor(w, ticks(0.4))                         // her view can't go back past her first frame's, tick 0
	alice.back = 0.375 * physics.ServerTickRateHz // she draws him from before he leaves

	k := fireAtEcho(t, w, alice)
	var ended []projectile.ProjectileState
	for i := 1; i <= k && len(ended) == 0; i++ {
		if i > k-ticks(0.35) {
			bob.move(0, 1)
		}
		w.Tick()
		ended = endedShots(t, w, "alice")
	}

	got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	if len(ended) != 1 || !touching(ended[0], drawn.X, drawn.Y, drawn.Radius) || got.HP != got.MaxHP {
		t.Fatalf("shots ended %+v and bob's Echo HP is %v, want one touching his Body where alice drew it, "+
			"(%v, %v), and HP %v", ended, got.HP, drawn.X, drawn.Y, got.MaxHP)
	}
}

// A Body standing on its own Echo touches the shot at the same moment, and wins the tie: the shot stops
// on the Body, and the Echo takes no damage.
func TestBodyOnItsEchoShieldsIt(t *testing.T) {
	w, alice, _ := duel(t)
	flyFor(w, fireAtEcho(t, w, alice))

	body := yourBody(t, bodiesFor(t, w, "bob"))
	got, ended := echoOf(t, w, "alice", body), endedShots(t, w, "alice")
	if len(ended) != 1 || !touching(ended[0], body.X, body.Y, body.Radius) || got.HP != got.MaxHP {
		t.Fatalf("shots ended %+v and bob's Echo HP is %v under his Body, want one touching it and HP %v",
			ended, got.HP, got.MaxHP)
	}
}

// A shot meets bob where alice drew him: he walked off its path after her view, and it still stops on
// him.
func TestShotHitsWhereTheViewHadIt(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, -1, ticks(0.5))            // off his Echo
	alice.back = 0.375 * physics.ServerTickRateHz // her view at step k is from before he leaves, 0.35 s before it
	from, to := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	alice.fire(to.X-from.X, to.Y-from.Y)
	w.Tick()
	k := firstTouch(t, w, alice, math.Hypot(to.X-from.X, to.Y-from.Y), to.Radius) // where he stood
	for i := 1; i <= k; i++ {
		if n := len(shots(t, w, "alice")); n != 1 {
			t.Fatalf("%d shots left after %d steps, want the shot until step %d", n, i, k)
		}
		if i > k-ticks(0.35) {
			bob.move(0, 1)
		}
		w.Tick()
	}
	if n := len(shots(t, w, "alice")); n != 0 {
		t.Fatalf("%d shots left, want the shot stopped where she saw bob", n)
	}
	if b := yourBody(t, bodiesFor(t, w, "bob")); b.Y-to.Y < 1.5 {
		t.Fatalf("bob moved %.2f, want him off the path now", b.Y-to.Y)
	}
	// His screen draws him off the path, so his sync says the shot ended on his Body, and where on it.
	if e := endedShots(t, w, "bob"); len(e) != 1 || !e[0].OnTarget || e[0].TargetSerial != to.Serial ||
		math.Abs(e[0].X-e[0].OffsetX-to.X) > 1e-9 || math.Abs(e[0].Y-e[0].OffsetY-to.Y) > 1e-9 {
		t.Fatalf("bob's sync ended %+v, want it on his Body (serial %d), offset from where she saw him, (%v, %v)",
			e, to.Serial, to.X, to.Y)
	}
}

// A view never goes back, so alice can't pick an older moment for each shot. After a frame drawn 0.075 s
// back, her shot sent seeing bob 0.375 s back, as in TestShotHitsWhereTheViewHadIt, is judged at the newer
// view, where he has walked off its path, and flies on. A new session or a view that isn't a number in
// between doesn't let her go back either.
func TestViewNeverGoesBack(t *testing.T) {
	for _, c := range []struct {
		name    string
		between func(w *cardinal.TestWorld, alice *client)
	}{
		{"one session", func(*cardinal.TestWorld, *client) {}},
		{"new session", func(_ *cardinal.TestWorld, alice *client) { alice.restart() }},
		{"after a view that isn't a number", func(w *cardinal.TestWorld, alice *client) {
			alice.back = math.NaN()
			alice.move(0, 0)
			w.Tick()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, alice, bob := duel(t)
			moveFor(w, bob, 0, -1, ticks(0.5)) // off his Echo
			alice.back = 0.075 * physics.ServerTickRateHz
			alice.move(0, 0)
			w.Tick()
			c.between(w, alice)
			alice.back = 0.375 * physics.ServerTickRateHz
			from, to := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
			alice.fire(to.X-from.X, to.Y-from.Y)
			w.Tick()
			k := firstTouch(t, w, alice, math.Hypot(to.X-from.X, to.Y-from.Y), to.Radius)
			for i := 1; i <= k; i++ {
				if i > k-ticks(0.35) {
					bob.move(0, 1)
				}
				w.Tick()
			}
			if n := len(shots(t, w, "alice")); n != 1 {
				t.Fatalf("%d shots left, want hers flying on past where the older view drew bob", n)
			}
		})
	}
}

// A shot meets bob where alice's screen drew him, partway between two of his rows: he dashes up across
// her shot's path, a unit a tick at any rate (test content). Drawn 0.3 of the way from the row on the
// path, he is hit there; drawn 0.7 of the way, the shot passes under him and flies on. A step before, she
// drew him a unit lower, 0.7 or 0.3 below the path, where the shot can't reach him yet.
func TestShotMeetsTargetBetweenRows(t *testing.T) {
	for _, c := range []struct {
		part float64
		hits bool
	}{{0.3, true}, {0.7, false}} {
		w := cardinal.NewTestWorld(t, withContent(t, map[string]string{"ability/tuning.json": fmt.Sprintf(
			`{"abilities": {"dash": {"params": {"distance": %v, "seconds": 0.3}}}}`, 0.3*physics.ServerTickRateHz)}))
		alice, bob := claim(t, w)
		moveFor(w, bob, -1, 0, ticks(0.4)) // off his Echo, toward her
		moveFor(w, bob, 0, -1, ticks(0.2)) // a unit below her shot's path
		from, to := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
		// Drawn 0.3 up, his circle first touches the path in step k; the dash cast at step 5 moves him
		// from step 6 on, so her view at step k is between his rows at steps 6 and 7.
		_, shot := primary(t)
		reach := to.Radius + shot.Radius
		k := int((to.X - from.X - math.Sqrt(reach*reach-0.3*0.3)) / (shot.Speed * physics.SecondsPerTick))
		alice.back = float64(k-6) - c.part
		alice.fire(1, 0)
		w.Tick()
		dashSlot, _ := dash(t)
		for i := 1; i <= k; i++ {
			if i == 5 {
				bob.cast(dashSlot, 0, 1)
			}
			w.Tick()
		}
		if gone := len(shots(t, w, "alice")) == 0; gone != c.hits {
			t.Errorf("drawn %v of the way: the shot stopped on bob: %v, want %v", c.part, gone, c.hits)
		}
	}
}

// A view the server keeps no rows for is held inside them, so alice's shot still meets bob's Echo, which
// stands still: one older than rewind_seconds (a high ping), one not saved yet, and one that isn't a
// number.
func TestViewOutsideTheRowsStillHits(t *testing.T) {
	keep := float64(tuningOf[projectile.ProjectileTuning](t).RewindTicks())
	for _, c := range []struct {
		name string
		back float64
	}{
		{"too old", keep + 3.5},
		{"not saved yet", 0.5},
		{"ahead of the server", -4},
		{"not a number", math.NaN()},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, alice, bob := duel(t)
			moveFor(w, bob, 0, 1, ticks(0.5)) // off his Echo
			alice.back = c.back
			flyFor(w, fireAtEcho(t, w, alice))
			got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
			if _, shot := primary(t); got.HP != got.MaxHP-shot.Damage {
				t.Fatalf("bob's Echo HP = %v, want %v", got.HP, got.MaxHP-shot.Damage)
			}
		})
	}
}

// A wall on the path in front of bob's Echo, now, stops the shot first, though alice drew the Echo
// behind it: the map isn't rewound.
func TestWallBeforeEchoStopsShot(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, ticks(0.5)) // off his Echo
	target := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	a := yourBody(t, bodiesFor(t, w, "alice"))
	// alice walks to 1.25 from the Echo, and a thin wall goes between.
	speed := bodiesFor(t, w, "alice").You.MoveSpeed * physics.SecondsPerTick
	moveFor(w, alice, 1, 0, int(math.Round((target.X-1.25-a.X)/speed)))
	a = yourBody(t, bodiesFor(t, w, "alice"))
	addWall(w, physics2d.Vec2{X: (a.X + target.X) / 2, Y: target.Y}, physics2d.Vec2{X: 0.05, Y: 2})
	w.Tick() // physics picks the wall up

	alice.fire(1, 0)
	_, shot := primary(t)
	flyFor(w, int(math.Ceil((target.X-a.X-target.Radius-shot.Radius)/(shot.Speed*physics.SecondsPerTick)))) // to the Echo
	got := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob")))
	if n := len(shots(t, w, "alice")); got.HP != got.MaxHP || n != 0 {
		t.Fatalf("bob's Echo HP = %v behind a wall with %d shots left, want %v and the shot stopped",
			got.HP, n, got.MaxHP)
	}
}

// A shot no frame cast, like an Echo's, meets Bodies and Echoes where they are now: no client predicts
// it.
func TestEchoShotHitsLive(t *testing.T) {
	w, _, bob, fx := fxDuel(t)
	moveFor(w, bob, 0, 1, ticks(0.5)) // off his Echo
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
	if len(ended) != 1 || !touching(ended[0], target.X, target.Y, target.Radius) || !ended[0].OnTarget ||
		ended[0].TargetSerial != target.Serial {
		t.Fatalf("shots ended at %+v, want one on bob's Echo (serial %d), touching it at (%v, %v)", ended,
			target.Serial, target.X, target.Y)
	}
}

// Shots pass allies: alice's shot flies through carol where she drew her.
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

	flyFor(w, int(math.Ceil(drawLag))+1) // carol's rows reach back to alice's view

	// The shot starts inside alice's own Body and Echo, then crosses carol.
	alice.fire(1, 0)
	w.Tick()
	_, shot := primary(t)
	flyFor(w, int(math.Ceil(2.5/(shot.Speed*physics.SecondsPerTick)))) // past her

	s := shots(t, w, "alice")
	if len(s) != 1 || s[0].X <= from.X+2.5 {
		t.Fatalf("shots = %+v, want one past carol (x > %.2f)", s, from.X+2.5)
	}
}

// A shot ends at its range in every direction, though a rounded speed can leave its last step short.
// Each is sent once more, at its range, its last tick flown from where the one before ended.
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
	flying := map[uint32]projectile.ProjectileState{}
	for _, s := range shots(t, w, "alice") {
		flying[s.EntityID] = s
	}
	if len(flying) != n {
		t.Fatalf("%d of %d shots left after %d of %d steps", len(flying), n, steps-1, steps)
	}
	ended := flyUntilEnded(t, w, "alice", 1)
	if got := len(shots(t, w, "alice")); got != 0 || len(ended) != n {
		t.Fatalf("%d shots still flying and %d ended after %d steps, want all %d ended", got, len(ended), steps, n)
	}
	for _, s := range ended {
		was := flying[s.EntityID]
		if d := math.Hypot(s.X-100, s.Y-100); math.Abs(d-shot.Range) > 1e-9 || !(s.Until > 0 && s.Until <= 1) ||
			s.PrevX != was.X || s.PrevY != was.Y {
			t.Fatalf("shot %d ended %v from its start, Until %v, from (%v, %v), want its range %v from (%v, %v)",
				s.EntityID, d, s.Until, s.PrevX, s.PrevY, shot.Range, was.X, was.Y)
		}
	}
}

// Cardinal gives a freed ID out again at once, so a shot launched the tick after another ended takes its
// ID. Every synced entity has its own serial, which the new shot doesn't share: a client keys each by
// it, so it never draws one from the other.
func TestNewShotTakesAnEndedOnesIDNotItsSerial(t *testing.T) {
	fx := &emitter{}
	w := cardinal.NewTestWorld(t, withEmitter(register, fx))
	claim(t, w)
	def, shot := primary(t)
	hook, _ := hooks.Bind(def.Hook, def.Params, physics.ServerTickRateHz) // primary checked it
	launch := func(k uint64) {
		emit(fx, projectilecomp.LaunchIntent{Shot: hook.Refs.Shots[0], Cause: abilitycomp.Cause{Tick: k},
			From: physics2d.Vec2{X: 100, Y: 100}, Dir: physics2d.Vec2{X: 1}}) // far from everyone
	}
	launch(1)
	w.Tick()
	first := shots(t, w, "alice")
	ended := flyUntilEnded(t, w, "alice", int(math.Ceil(shot.Range/(shot.Speed*physics.SecondsPerTick))))
	launch(2)
	w.Tick()
	next := shots(t, w, "alice")
	if len(first) != 1 || len(ended) != 1 || len(next) != 1 {
		t.Fatalf("%d shots, then %d ended, then %d, want one each", len(first), len(ended), len(next))
	}
	if next[0].EntityID != first[0].EntityID {
		t.Fatalf("the new shot is entity %d, not the ended one's %d: Cardinal no longer reuses IDs at once",
			next[0].EntityID, first[0].EntityID)
	}
	serials := map[uint64]bool{ended[0].Serial: true}
	for _, b := range bodiesFor(t, w, "alice").Bodies.All() {
		serials[b.Serial] = true
	}
	for _, e := range sentTo[echo.EchoSync](t, w, "alice").Echoes.All() {
		serials[e.Serial] = true
	}
	if ended[0].Serial != first[0].Serial || serials[next[0].Serial] || serials[0] || len(serials) != 5 {
		t.Fatalf("serials: shot %d, ended %d, new %d; with Bodies and Echoes %v, want 5 others, none 0",
			first[0].Serial, ended[0].Serial, next[0].Serial, serials)
	}
}

func TestWallStopsShot(t *testing.T) {
	w, alice, bob := duel(t)
	moveFor(w, bob, 0, 1, ticks(0.5)) // off his Echo, so only the wall can stop the shot

	// A wall halfway between them.
	a, b := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	wallX := (a.X + b.X) / 2
	addWall(w, physics2d.Vec2{X: wallX, Y: 0}, physics2d.Vec2{X: 0.5, Y: 5})
	w.Tick() // physics picks the wall up

	ended := flyUntilEnded(t, w, "alice", fireAtEcho(t, w, alice)+2)
	// Sent once more, touching the wall's face, half its width from its center: on the map, not a target.
	if len(ended) != 1 || math.Abs(math.Abs(ended[0].X-wallX)-(0.5+ended[0].Radius)) > 2*box2d.LinearSlop ||
		ended[0].OnTarget {
		t.Fatalf("shots ended at %+v, want one touching the wall's face, on no target", ended)
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
	moveFor(w, bob, 0, 1, ticks(0.5)) // off his Echo
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
		if len(xs) > ticks(10) {
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

// Each Body and Echo keeps the rows every view rewind_seconds back is drawn from (KeptRows), one per tick,
// and its newest row is what this tick's syncs show of it: what its screen draws, and the target_sync its
// predictor places it by, as the server's shots meet it.
func TestTargetHistoryIsWhatSyncsShow(t *testing.T) {
	w, _, bob, fx := fxDuel(t)
	fx.effect(yourBody(t, bodiesFor(t, w, "bob")), effectcomp.EffectIntent{Kind: "stun", Ticks: 3})
	keep := int(tuningOf[projectile.ProjectileTuning](t).KeptRows())
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
			side := interact.Side{Owner: b.EntityID, Team: b.Team}
			check(fmt.Sprintf("Body %d", b.EntityID), newest(b.EntityID), flight.Row{Tick: bodies.Tick, Kind: interact.Body,
				Side: side, Center: box2d.Vec2{X: b.X, Y: b.Y}, Radius: b.Radius, Tags: b.Tags, Until: 1}, i >= keep)
		}
		for e := range echoes.Echoes.Values() {
			side := interact.Side{Owner: e.Owner, Team: e.Team}
			check(fmt.Sprintf("Echo %d", e.EntityID), newest(e.EntityID), flight.Row{Tick: echoes.Tick, Kind: interact.Echo,
				Side: side, Center: box2d.Vec2{X: e.X, Y: e.Y}, Radius: e.Radius, Tags: e.Tags, Until: e.Until}, i >= keep)
			if owner := w.Entity(cardinal.EntityID(e.EntityID)).Get[team.Owner]().Body; uint32(owner) != e.Owner {
				t.Fatalf("Echo %d says its owner is %d, not %d", e.EntityID, e.Owner, owner)
			}
		}
		targets := sentTo[projectile.TargetSync](t, w, "alice")
		if targets.Tick != bodies.Tick || targets.Targets.Len() != bodies.Bodies.Len()+echoes.Echoes.Len() {
			t.Fatalf("target_sync %+v, want tick %d's %d Bodies and Echoes", targets, bodies.Tick,
				bodies.Bodies.Len()+echoes.Echoes.Len())
		}
		for s := range targets.Targets.Values() {
			rows := newest(s.EntityID)
			r := rows[len(rows)-1]
			want := projectile.TargetState{EntityID: s.EntityID, Serial: s.Serial, Kind: r.Kind, Owner: r.Side.Owner,
				Team: r.Side.Team, X: r.Center.X, Y: r.Center.Y, Radius: r.Radius, Tags: r.Tags, Until: r.Until}
			if s != want || s.Serial != w.Entity(cardinal.EntityID(s.EntityID)).Get[ref.Spawn]().Serial {
				t.Fatalf("target_sync sent %+v, want its newest row, %+v, and its serial", s, want)
			}
		}
	}
	if !stunned {
		t.Fatal("no sync showed bob stunned")
	}
}

// shootEcho has alice fire kit slot 0 at bob's Echo, and ticks until the shot is gone. It returns his
// Echo's HP then.
func shootEcho(t *testing.T, w *cardinal.TestWorld, alice *client) float64 {
	t.Helper()
	from, bob := yourBody(t, bodiesFor(t, w, "alice")), yourBody(t, bodiesFor(t, w, "bob"))
	to := echoOf(t, w, "alice", bob)
	alice.fire(to.X-from.X, to.Y-from.Y)
	w.Tick()
	for range ticks(5) {
		if len(shots(t, w, "alice")) == 0 {
			return echoOf(t, w, "alice", bob).HP
		}
		w.Tick()
	}
	t.Fatal("the shot is still flying after 5 s")
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
	moveFor(w, bob, -1, 0, ticks(0.1)) // half over his Echo, toward alice
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
	moveFor(w, bob, -1, 0, ticks(0.1)) // half over his Echo, toward alice
	full := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))).MaxHP
	if hp := shootEcho(t, w, alice); hp != full {
		t.Fatalf("bob's Echo HP = %v behind his Body, want %v: it blocks while not stunned", hp, full)
	}
	fx.effect(yourBody(t, bodiesFor(t, w, "bob")), effectcomp.EffectIntent{Kind: "stun", Ticks: uint64(ticks(5))})
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

// body_sync tells you where your Echo will be: its spot this tick, then each next tick's, as far as
// echo_path_seconds and its delay reach. It then goes there, whatever your Body does now (test content:
// an Echo that trails by 2 s, and one by 0.1 s).
func TestYouStateHasYourEchosPath(t *testing.T) {
	ahead := tuningOf[player.MovementTuning](t).EchoPathTicks()
	for _, c := range []struct {
		name  string
		delay float64 // seconds
	}{
		{"long", 2},
		{"short", 0.1},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := cardinal.NewTestWorld(t, withContent(t, map[string]string{
				"echo/tuning.json": fmt.Sprintf(`{"champions": {"starter": {"delay_seconds": %v}}}`, c.delay),
			}))
			alice, _ := claim(t, w)
			delay := ticks(c.delay)
			for i := range delay + 5 { // a curve, until the Echo follows it
				alice.move(math.Cos(float64(i)/7), math.Sin(float64(i)/7))
				w.Tick()
			}
			s := bodiesFor(t, w, "alice")
			path := slices.Collect(s.You.EchoPath.Values())
			if int(s.You.EchoDelay) != delay || len(path) != min(ahead, delay)+1 {
				t.Fatalf("EchoDelay %d with %d spots, want %d with %d", s.You.EchoDelay, len(path), delay,
					min(ahead, delay)+1)
			}
			body := yourBody(t, s)
			for k, want := range path {
				if k > 0 {
					alice.move(-1, 0)
					w.Tick()
				}
				if e := echoOf(t, w, "alice", body); e.X != want.X || e.Y != want.Y {
					t.Fatalf("%d ticks on: Echo at (%v, %v), want (%v, %v)", k, e.X, e.Y, want.X, want.Y)
				}
			}
		})
	}
}

// Your predictor draws your Echo where the server will have it at your predicted tick: along the path
// body_sync sends, then where it predicted your Body, once that is the Echo's delay back. She walks a
// curve first, so her Echo walks during the frames. Ten frames from one state, as when acks lag, land on
// the server's Echo with an Echo 0.1 s behind. With a path shorter than that lead, your Echo waits at
// the path's end (test content).
func TestPredictedEchoMatchesServer(t *testing.T) {
	rules, err := predict.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name        string
		delay, path float64 // seconds
		exact       int     // frames it predicts exactly, of ten
	}{
		{"short delay", 0.1, 0.5, 10},
		{"lead past the path", 2, 0.1, ticks(0.1)}, // as far as the path goes
	} {
		t.Run(c.name, func(t *testing.T) {
			w := cardinal.NewTestWorld(t, withContent(t, map[string]string{
				"echo/tuning.json":   fmt.Sprintf(`{"champions": {"starter": {"delay_seconds": %v}}}`, c.delay),
				"player/tuning.json": fmt.Sprintf(`{"echo_path_seconds": %v}`, c.path),
			}))
			alice, _ := claim(t, w)
			for i := range ticks(c.delay) + 5 { // a curve, until her Echo follows it
				alice.move(math.Cos(float64(i)/7), math.Sin(float64(i)/7))
				w.Tick()
			}
			p := predict.New(rules)
			p.Reset(predictFrom(t, bodiesFor(t, w, "alice")))
			var first, end box2d.Vec2 // the server's Echo after the first frame, and at the path's end
			for i := range 10 {
				f := alice.send(playercomp.InputFrame{MoveX: math.Cos(float64(i)), MoveY: math.Sin(float64(i))})
				w.Tick()
				p.Step(predictFrame(f))
				e, ok := ownEcho(sentTo[echo.EchoSync](t, w, "alice"), bodiesFor(t, w, "alice").You.Body)
				want := box2d.Vec2{X: e.X, Y: e.Y}
				if i == 0 {
					first = want
				}
				if i == c.exact-1 {
					end = want
				}
				if i >= c.exact {
					want = end
				}
				if at, _, predicted := p.Echo(); !ok || !predicted || at != want {
					t.Fatalf("frame %d: predicted her Echo at %v (%v), want %v (%v)", i+1, at, predicted, want, ok)
				}
			}
			if end == first {
				t.Fatalf("her Echo stood at %v, so the frames show nothing", end)
			}
		})
	}
}

// Your screen draws your Echo where your predictor has it, now, as it does your Body, so a shot that
// stops on it meets it there, on the server and in your predictor alike (test content: a bolt that stops
// on its caster's Echo). Her Echo walks toward her along her shot's path, and her view had it 0.125 s
// further back. Her bolt's hit is offset from where it is now.
func TestShotMeetsYourEchoWhereYouDrawIt(t *testing.T) {
	files := patched(t, map[string]string{
		"echo/tuning.json": `{"champions": {"starter": {"delay_seconds": 1}}}`,
		"projectile/tuning.json": `{"shots": {"bolt": {"meets": [
			{"kinds": ["echo"], "relations": ["own"], "do": "stop"}]}}}`,
	})
	w := cardinal.NewTestWorld(t, func(world *cardinal.World) { registerWith(world, files) })
	alice, _ := claim(t, w)
	moveFor(w, alice, 1, 0, physics.ServerTickRateHz) // her Echo walks this a second later
	flyFor(w, ticks(0.25))
	rules, err := predict.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	p := predict.New(rules)
	body := yourBody(t, bodiesFor(t, w, "alice")).EntityID
	var ended []predict.Ended
	for i := 0; len(ended) == 0; i++ {
		if i == ticks(0.5) {
			t.Fatal("her bolt never met her Echo")
		}
		before := bodiesFor(t, w, "alice")
		see(t, p, sentTo[projectile.TargetSync](t, w, "alice"))
		p.Reset(predictFrom(t, before))
		f := playercomp.InputFrame{AimX: -1}
		if i == 0 {
			f.Buttons = 1
		}
		f = alice.next(f)
		_, ended = p.Step(predictFrame(f))
		alice.send(f)
		w.Tick()
		requireEnded(t, fmt.Sprintf("step %d", i), ended, w.Emitted[projectilecomp.ShotEnded](), body)
	}
	e, _ := ownEcho(sentTo[echo.EchoSync](t, w, "alice"), body)
	if end := ended[0].End; end.Hit.Target != e.EntityID || math.Abs(end.At.X-end.Hit.Offset.X-e.X) > 1e-9 ||
		math.Abs(end.At.Y-end.Hit.Offset.Y-e.Y) > 1e-9 {
		t.Fatalf("her bolt ended %+v, want on her Echo where it is now, %+v", ended[0], e)
	}
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
	away := ticks(1)
	moveFor(w, alice, 0, 1, away) // 5 m away, so her Echo stays on her spawn until the delay is over
	moveFor(w, bob, -1, 0, walk)  // onto alice's Echo
	for i := away + walk + 3; i <= delay+5; i++ {
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
	// She lands clear of the wall, on the side she came from, within one 0.25 m grid step of it (and a
	// rounding: steps of 1/60 s aren't exact).
	b := yourBody(t, bodiesFor(t, w, "alice"))
	if edge := b.X + radiusOf(w, a); !(edge < face && edge >= face-0.25-1e-9) || b.Y != a.Y {
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
		fx.effect(a, effectcomp.EffectIntent{Kind: "phase", Ticks: uint64(ticks(0.6))}) // ends inside the wall
		fx.effect(a, effectcomp.EffectIntent{Kind: "slow", Ticks: uint64(ticks(1.5)), Value: -2500, Source: bID})
		slot, _ := dash(t)
		bob.cast(slot, -0.6, 0.8)
		w.Tick()
		for i := range ticks(1.5) {
			if i == ticks(0.15) {
				fx.effect(b, effectcomp.EffectIntent{Kind: "stun", Ticks: uint64(ticks(0.2)), Source: aID})
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

// body_sync says which sim the shard runs, so a client predicts only with a predictor of the same
// (predictor.mjs stamps its wasm from the same function: predictor.test.ts).
func TestBodySyncSaysWhichSimItRuns(t *testing.T) {
	w := cardinal.NewTestWorld(t, register)
	w.Command("alice", input(1, frame(1, 0, 0)))
	w.Tick()
	if got, want := bodiesFor(t, w, "alice").SimVersion, echoimage.SimVersion(physics.ServerTickRateHz); got != want {
		t.Fatalf("body_sync says sim %q, want %q", got, want)
	}
}

// predictionStep is one tick of TestPredictionMatchesServer's run: the body_sync and target_sync before
// it, the frames her client stepped from them (the one the server applied, or on the bad network every
// one unacked, maybe none), and what the predictor gave after, as predictor.wasm returns it.
type predictionStep struct {
	Sync    player.BodySync
	Targets projectile.TargetSync
	Frames  []playercomp.InputFrame
	X, Y    float64          // your Body
	Shots   []shotPrediction // your shots, in the predictor's order
	Ended   []endedPrediction
	Kit     kitPrediction
	Echo    *echoPrediction // nil with no Echo
}

// digest is s as a line of text, so a new recording shows what changed in a reviewed diff.
func (s predictionStep) digest() string {
	echo := "none"
	if s.Echo != nil {
		echo = fmt.Sprintf("%+v", *s.Echo)
	}
	return fmt.Sprintf("tick %d frames %+v: body %v,%v shots %+v ended %+v kit %+v echo %s\n", s.Sync.Tick, s.Frames,
		s.X, s.Y, s.Shots, s.Ended, s.Kit, echo)
}

// echoPrediction is your Echo, as predictor.wasm's echo() returns it.
type echoPrediction struct {
	X, Y, PrevX, PrevY float64
}

// shotPrediction is one of your shots, as predictor.wasm's shots() returns it.
type shotPrediction struct {
	Type                       string
	Seq                        uint32
	Ability                    string
	Launch                     uint32
	X, Y, PrevX, PrevY, Radius float64
}

// endedPrediction is one of your shots that ended in a step, as predictor.wasm's step() returns it.
type endedPrediction struct {
	Type                              string
	Seq                               uint32
	Ability                           string
	Launch                            uint32
	X, Y, PrevX, PrevY, Radius, Until float64
	OnTarget                          bool
	TargetSerial                      uint64
	OffsetX, OffsetY                  float64
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
		Tick: got.Tick, Body: you.Body, Team: b.Team, Center: box2d.Vec2{X: b.X, Y: b.Y},
		Radius: b.Radius, MoveSpeed: you.MoveSpeed, Champion: you.Champion,
		Cooldowns: slices.Collect(you.Cooldowns.Values()), Effects: slices.Collect(you.Effects.Values()),
		EffectsNext: you.EffectsNext, EchoDelay: you.EchoDelay,
	}
	for at := range you.EchoPath.Values() {
		state.EchoPath = append(state.EchoPath, box2d.Vec2(at))
	}
	for shot := range you.Shots.Values() {
		key := predict.Key{Seq: shot.Seq, Ability: shot.Ability, Launch: shot.Launch}
		state.Shots = append(state.Shots, predict.Shot{Type: shot.Type, Key: key, Flight: flight.Flight{
			Pos:      box2d.Vec2{X: shot.X, Y: shot.Y},
			Velocity: box2d.Vec2{X: shot.VelX, Y: shot.VelY}, Radius: shot.Radius, Remaining: shot.Remaining,
		}, Prev: box2d.Vec2{X: shot.PrevX, Y: shot.PrevY}})
	}
	return state
}

// see gives p the targets a client's target_sync shows, as predictor.ts does.
func see(t *testing.T, p *predict.Predictor, targets projectile.TargetSync) {
	t.Helper()
	var sights []predict.Sight
	for s := range overWire(t, targets).Targets.Values() {
		sights = append(sights, predict.Sight{ID: s.EntityID, Serial: s.Serial, Row: flight.Row{
			Kind: s.Kind, Side: interact.Side{Owner: s.Owner, Team: s.Team}, Center: box2d.Vec2{X: s.X, Y: s.Y},
			Radius: s.Radius, Tags: s.Tags, Until: s.Until,
		}})
	}
	p.See(targets.Tick, sights)
}

// ownEcho is the Echo of Body body in echoes.
func ownEcho(echoes echo.EchoSync, body uint32) (echo.EchoState, bool) {
	for e := range echoes.Echoes.Values() {
		if e.Owner == body {
			return e, true
		}
	}
	return echo.EchoState{}, false
}

// predictFrame is f as the predictor takes it.
func predictFrame(f playercomp.InputFrame) predict.Frame {
	return predict.Frame{
		Seq: f.Seq, MoveX: f.MoveX, MoveY: f.MoveY, AimX: f.AimX, AimY: f.AimY, Buttons: f.Buttons, ViewTick: f.ViewTick,
	}
}

// requireEnded fails unless ended, the shots a step ended, are the ends the server gave body's predicted
// shots this tick, to the bit.
func requireEnded(t *testing.T, at string, ended []predict.Ended, server []projectilecomp.ShotEnded, body uint32) {
	t.Helper()
	type end struct {
		Seq     uint32
		Ability string
		End     flight.End
	}
	var got, want []end
	for _, e := range ended {
		got = append(got, end{e.Seq, e.Ability, e.End})
	}
	for _, e := range server {
		if e.Predicted && uint32(e.Cause.Caster) == body {
			want = append(want, end{e.Cause.Seq, e.Cause.Ability, e.End})
		}
	}
	order := func(a, b end) int {
		return cmp.Or(cmp.Compare(a.Seq, b.Seq), cmp.Compare(a.Ability, b.Ability),
			cmp.Compare(a.End.At.X, b.End.At.X), cmp.Compare(a.End.At.Y, b.End.At.Y))
	}
	slices.SortFunc(got, order)
	slices.SortFunc(want, order)
	if !slices.Equal(got, want) {
		t.Fatalf("%s: predicted ends %+v, the server's %+v", at, got, want)
	}
}

// guess is what a predictor gave after a step: its state, and where it has your Echo.
type guess struct {
	state   predict.State
	echo    box2d.Vec2
	hasEcho bool
}

// guessOf is p's guess after its last step.
func guessOf(p *predict.Predictor) guess {
	at, _, ok := p.Echo()
	return guess{state: p.State(), echo: at, hasEcho: ok}
}

// requirePredicted fails unless g is the server's state in after and echoes: her Body, its Echo,
// cooldowns and shots, and her effects unless something else put one on her this tick.
func requirePredicted(t *testing.T, at string, g guess, after player.BodySync, echoes echo.EchoSync, effects bool) {
	t.Helper()
	got, want := g.state, predictFrom(t, after)
	if got.Center != want.Center {
		t.Fatalf("%s: predicted her Body at %v, the server has %v", at, got.Center, want.Center)
	}
	if e, found := ownEcho(echoes, after.You.Body); g.hasEcho != found || found && g.echo != (box2d.Vec2{X: e.X, Y: e.Y}) {
		t.Fatalf("%s: predicted her Echo at %v (%v), the server has %+v (%v)", at, g.echo, g.hasEcho, e, found)
	}
	if !slices.Equal(got.Cooldowns, want.Cooldowns) {
		t.Fatalf("%s: predicted cooldowns %+v, the server has %+v", at, got.Cooldowns, want.Cooldowns)
	}
	if effects && (!slices.Equal(got.Effects, want.Effects) || got.EffectsNext != want.EffectsNext) {
		t.Fatalf("%s: predicted effects %+v (next %d), the server has %+v (next %d)",
			at, got.Effects, got.EffectsNext, want.Effects, want.EffectsNext)
	}
	// The server lists shots in no set order.
	sorted := func(shots []predict.Shot) []predict.Shot {
		out := slices.Clone(shots)
		slices.SortFunc(out, func(a, b predict.Shot) int {
			return cmp.Or(cmp.Compare(a.Pos.X, b.Pos.X), cmp.Compare(a.Pos.Y, b.Pos.Y))
		})
		return out
	}
	if g, w := sorted(got.Shots), sorted(want.Shots); !slices.Equal(g, w) {
		t.Fatalf("%s: predicted shots %+v, the server has %+v", at, g, w)
	}
}

// record is p's state as predictor.wasm returns it, for the recording.
func record(p *predict.Predictor, sync player.BodySync, targets projectile.TargetSync, frames []playercomp.InputFrame,
	ended []predict.Ended,
) predictionStep {
	s := p.State()
	step := predictionStep{
		Sync: sync, Targets: targets, Frames: append([]playercomp.InputFrame{}, frames...), X: s.Center.X, Y: s.Center.Y,
		Shots: []shotPrediction{},
		Ended: []endedPrediction{}, Kit: kitPrediction{Tick: float64(s.Tick), Slots: []kitSlot{}},
	}
	for _, shot := range s.Shots {
		step.Shots = append(step.Shots, shotPrediction{
			Type: shot.Type, Seq: shot.Seq, Ability: shot.Ability, Launch: shot.Launch,
			X: shot.Pos.X, Y: shot.Pos.Y, PrevX: shot.Prev.X, PrevY: shot.Prev.Y, Radius: shot.Radius,
		})
	}
	for _, e := range ended {
		hit := e.End.Hit
		step.Ended = append(step.Ended, endedPrediction{
			Type: e.Type, Seq: e.Seq, Ability: e.Ability, Launch: e.Launch,
			X: e.End.At.X, Y: e.End.At.Y, PrevX: e.Pos.X, PrevY: e.Pos.Y, Radius: e.Radius, Until: e.End.Until,
			OnTarget: hit.OnTarget(), TargetSerial: hit.Serial, OffsetX: hit.Offset.X, OffsetY: hit.Offset.Y,
		})
	}
	for _, c := range p.Kit() {
		step.Kit.Slots = append(step.Kit.Slots, kitSlot{Ability: c.Ability, ReadyTick: float64(c.ReadyTick)})
	}
	if at, prev, ok := p.Echo(); ok {
		step.Echo = &echoPrediction{X: at.X, Y: at.Y, PrevX: prev.X, PrevY: prev.Y}
	}
	return step
}

// The client predicts its Body with internal/predict, from what body_sync sends: its moves, its casts
// and their cooldowns, its dash, its shots and its Echo. Started from any tick's state and stepped by the
// frames after it, it lands on the server's bits, through slows, stuns, forced moves, held buttons, bad
// frames and a shot flying out its range. Her second bolt meets bob, walking across it, where her screen
// drew him: the server ends it the same tick at the same point, and a replay of the frame ends it there
// too. Effects others put on her surprise it, and come with the next state. Her Echo trails her by a
// second (test content), so it retraces her path in the run.
// testdata/prediction-20hz.msgpack (one per tick rate) keeps the run and the bad network's replays, so the
// client checks that its wasm build lands there too (client/src/game/player/predictor.test.ts).
func TestPredictionMatchesServer(t *testing.T) {
	fx := &emitter{}
	w := cardinal.NewTestWorld(t, withEmitter(withContent(t, map[string]string{
		"echo/tuning.json": `{"champions": {"starter": {"delay_seconds": 1}}}`,
	}), fx))
	alice, bob := claim(t, w)
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
	alice.back = lag
	walk := bodiesFor(t, w, "bob").You.MoveSpeed * physics.SecondsPerTick

	// One frame from each tick's state, as when the server acks every frame, for two cooldowns. Her
	// client sees each tick's syncs, steps the frame from the newest state, and sends it.
	var run []predictionStep
	var ends []predict.Ended
	for i := range 2 * second {
		e, surprised := surprises[i]
		if surprised {
			fx.effect(a, e)
		}
		if i >= second-6 {
			bob.move(0, 1) // up across her second bolt's path
		}
		before, targets := bodiesFor(t, w, "alice"), sentTo[projectile.TargetSync](t, w, "alice")
		see(t, p, targets)
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
		f = alice.next(f)
		_, ended := p.Step(predictFrame(f))
		ends = append(ends, ended...)
		alice.send(f)
		w.Tick()
		after, afterEchoes, at := bodiesFor(t, w, "alice"), sentTo[echo.EchoSync](t, w, "alice"), fmt.Sprintf("step %d", i)
		requirePredicted(t, at, guessOf(p), after, afterEchoes, !surprised)
		requireEnded(t, at, ended, w.Emitted[projectilecomp.ShotEnded](), a.EntityID)
		run = append(run, record(p, before, targets, []playercomp.InputFrame{f}, ended))
		p.Reset(predictFrom(t, before))
		_, replayed := p.Step(predictFrame(f))
		requirePredicted(t, at+" replayed", guessOf(p), after, afterEchoes, !surprised)
		requireEnded(t, at+" replayed", replayed, w.Emitted[projectilecomp.ShotEnded](), a.EntityID)
	}
	if len(run[0].Shots) != 1 || len(run[second-1].Shots) != 0 {
		t.Fatalf("predicted %d shots after her first frame and %d before her second bolt, want one, then none",
			len(run[0].Shots), len(run[second-1].Shots))
	}
	if first, last := run[0].Echo, run[len(run)-1].Echo; first == nil || last == nil || *first == *last {
		t.Fatalf("predicted her Echo at %v after her first frame and %v after her last, want it moved", first, last)
	}
	// Her first bolt is drawn out to its range and her second up to bob, each at its speed in its last tick.
	launch := box2d.Vec2{X: run[0].Shots[0].PrevX, Y: run[0].Shots[0].PrevY}
	bobBody := yourBody(t, bodiesFor(t, w, "bob")).EntityID
	if len(ends) != 2 || math.Abs(math.Hypot(ends[0].End.At.X-launch.X, ends[0].End.At.Y-launch.Y)-bolt.Range) > 1e-9 ||
		ends[1].End.Hit.Target != bobBody {
		t.Fatalf("ended %+v, want her first bolt at its range from %v, then her second on bob's Body %d",
			ends, launch, bobBody)
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
		requirePredicted(t, fmt.Sprintf("replayed frame %d", i+1), guessOf(p), bodiesFor(t, w, "alice"),
			sentTo[echo.EchoSync](t, w, "alice"), true)
		if shots := len(p.State().Shots); shots != 0 && i < 4 || shots != 1 && i >= 4 {
			t.Fatalf("replayed frame %d: %d shots predicted, want her bolt from frame 5 only", i+1, shots)
		}
	}

	run = append(run, badNetwork(t, w, p, alice)...)

	// The recording must still predict its own results, so a change to the sim asks for a new one. Its
	// digest is text, a line a step, so a new recording is a reviewed diff. Each tick rate has its own.
	name := filepath.Join("testdata", fmt.Sprintf("prediction-%dhz", physics.ServerTickRateHz))
	path, digestPath := name+".msgpack", name+".txt"
	digest := func(run []predictionStep) []byte {
		var b strings.Builder
		for _, s := range run {
			b.WriteString(s.digest())
		}
		return []byte(b.String())
	}
	if *update {
		raw, err := msgpack.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(digestPath, digest(run), 0o644); err != nil {
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
	if text, err := os.ReadFile(digestPath); err != nil || !bytes.Equal(text, digest(recorded)) {
		t.Fatalf("%s isn't the digest of %s (%v): run with -update to write both", digestPath, path, err)
	}
	p.Forget()
	for i, s := range recorded {
		see(t, p, s.Targets)
		p.Reset(predictFrom(t, s.Sync))
		var ended []predict.Ended
		for _, f := range s.Frames {
			_, ended = p.Step(predictFrame(f))
		}
		got := record(p, s.Sync, s.Targets, s.Frames, ended)
		if got.X != s.X || got.Y != s.Y || !slices.Equal(got.Shots, s.Shots) || got.Kit.Tick != s.Kit.Tick ||
			!slices.Equal(got.Kit.Slots, s.Kit.Slots) || !slices.Equal(got.Ended, s.Ended) ||
			(got.Echo == nil) != (s.Echo == nil) || got.Echo != nil && *got.Echo != *s.Echo {
			t.Fatalf("%s step %d: predicted (%v, %v), shots %+v, ended %+v, kit %+v, Echo %+v; recorded (%v, %v), %+v, "+
				"%+v, %+v, %+v. If the sim changed on purpose, run "+
				"`go test ./shards/game -run TestPredictionMatchesServer -update` (with -tags rate60 at 60 Hz)",
				path, i, got.X, got.Y, got.Shots, got.Ended, got.Kit, got.Echo, s.X, s.Y, s.Shots, s.Ended, s.Kit, s.Echo)
		}
	}
}

// badNetwork plays alice's client through a scripted network, holding fire at bob as she walks. Each
// tick of the script: '.' her client makes a frame and sends it, '0' its clock runs slow and makes none,
// '2' it makes two after a hitch, 'l' the frame's Input is lost (the next one resends it), and 'h' the
// network holds it until the next '.' or '2'. Two more held in a row than the server's queue holds
// overflow it, and the slow ticks after drain it before her bolt is ready again. Each tick her client
// starts over from the newest body_sync and replays her unacked frames, as it does. Whenever the server
// then applies those frames a tick each, it lands where the replay did, to the bit, its shots' ends
// included. A correction comes only from a surprise: at most a tick of walking for each tick the server
// coasted or frame it dropped. It returns each tick's replay, for the recording.
func badNetwork(t *testing.T, w *cardinal.TestWorld, p *predict.Predictor, alice *client) []predictionStep {
	t.Helper()
	queue, gap := tuningOf[player.MovementTuning](t).BufferFrames(), strings.Repeat(".", ticks(0.2))
	script := strings.Repeat(".", ticks(0.3)) + strings.Repeat("h", queue+2) + "." + strings.Repeat("0", queue-1) +
		strings.Repeat(".", ticks(0.3)) + "l" + gap + "2" + gap + "l" + gap + "0" + gap + "0" + gap + "h.h" +
		strings.Repeat(".", ticks(0.6))
	resend := ticks(0.4) // frames each Input repeats: ResendSeconds in client/src/game/player/input_stream.ts
	moves := [][2]float64{{1, 0}, {0.6, 0.8}, {0, 1}, {-1, 0}, {0, -1}, {1, 1}}
	before, targets := bodiesFor(t, w, "alice"), sentTo[projectile.TargetSync](t, w, "alice")
	walk, me := before.You.MoveSpeed*physics.SecondsPerTick, before.You.Body
	type replay struct { // a replay's guess, until the server applies its frame
		seq, ack uint32
		from     uint64 // the body_sync's tick it started from
		guess    guess
		ended    []predict.Ended
	}
	var (
		run                                           []predictionStep
		open                                          []replay
		unacked                                       []playercomp.InputFrame
		held                                          []player.Input
		drawn                                         box2d.Vec2 // her Body after her newest frame, as last predicted
		surprises                                     int        // since her client last predicted
		exact, deep, hits, coasts, dropped, corrected int
	)
	for i, c := range script {
		at := fmt.Sprintf("bad network tick %d", i)
		var ended []predict.Ended // her client's last step's
		// Her client starts over from the newest body_sync and replays what the server hasn't applied.
		see(t, p, targets)
		p.Reset(predictFrom(t, before))
		ack := before.You.InputAck
		unacked = slices.DeleteFunc(unacked, func(f playercomp.InputFrame) bool { return f.Seq <= ack })
		for _, f := range unacked {
			_, ended = p.Step(predictFrame(f))
			open = append(open, replay{f.Seq, ack, before.Tick, guessOf(p), ended})
		}
		if i > 0 {
			moved := box2d.Length(box2d.Sub(p.State().Center, drawn))
			if moved > 0 {
				corrected++
			}
			if moved > float64(surprises)*walk+1e-9 { // positions differ by sums of steps, so allow rounding
				t.Fatalf("%s: a correction of %v after %d surprises, past a walk (%v) each", at, moved, surprises, walk)
			}
		}
		// Her clock makes this tick's frames, each sent with the newest unacked ones.
		frames := map[rune]int{'0': 0, '2': 2}[c]
		if c != '0' && c != '2' {
			frames = 1
		}
		for range frames {
			m, bob := moves[int(alice.seq)%len(moves)], yourBody(t, bodiesFor(t, w, "bob"))
			from := p.Peek(m[0], m[1])
			f := alice.next(playercomp.InputFrame{MoveX: m[0], MoveY: m[1], AimX: bob.X - from.X, AimY: bob.Y - from.Y,
				Buttons: 1})
			alice.seq = f.Seq
			_, ended = p.Step(predictFrame(f))
			open = append(open, replay{f.Seq, ack, before.Tick, guessOf(p), ended})
			unacked = append(unacked, f)
			if c != 'l' {
				held = append(held, input(alice.session, unacked[max(len(unacked)-resend, 0):]...))
			}
			if c == '.' || c == '2' {
				for _, in := range held {
					w.Command(alice.id, in)
				}
				held = nil
			}
		}
		drawn = p.State().Center
		run = append(run, record(p, before, targets, unacked, ended))
		w.Tick()
		after := bodiesFor(t, w, "alice")
		applied := after.You.InputAck
		// What surprised her client: a tick the server coasted, or frames it dropped.
		surprises = max(int(applied-ack)-1, 0)
		if applied == ack {
			coasts++
			surprises = 1
		}
		dropped += max(int(applied-ack)-1, 0)
		// A replay from before a surprise is stale; any other is the server's state once its frame applies.
		echoes, ends := sentTo[echo.EchoSync](t, w, "alice"), w.Emitted[projectilecomp.ShotEnded]()
		waiting := open[:0]
		for _, r := range open {
			switch {
			case uint64(applied-r.ack) != after.Tick-r.from:
			case applied == r.seq:
				requirePredicted(t, at, r.guess, after, echoes, true)
				requireEnded(t, at, r.ended, ends, me)
				exact++
				if r.seq-r.ack > 1 {
					deep++
				}
				if slices.ContainsFunc(r.ended, func(e predict.Ended) bool { return e.End.Hit.OnTarget() }) {
					hits++
				}
			default:
				waiting = append(waiting, r)
			}
		}
		open = waiting
		before, targets = after, sentTo[projectile.TargetSync](t, w, "alice")
	}
	if exact < len(script) || deep == 0 || hits == 0 || coasts < 3 || dropped == 0 || corrected == 0 {
		t.Fatalf("bad network: %d exact replays, %d of several frames, %d ending on bob; %d coasts, %d frames "+
			"dropped, %d corrections: want each", exact, deep, hits, coasts, dropped, corrected)
	}
	return run
}

// The predictor places a target only from the target syncs of both ticks around the view the server
// judges a frame's shots at, and only while the newest sync shows it alive. Otherwise her shot passes
// where her screen drew it. A hit ends her shot touching it there, its last tick at its speed (test
// content: a bolt that flies 0.75 units a tick at any rate).
func TestPredictorPlacesOnlyWhatSyncsShow(t *testing.T) {
	_, bolt := primary(t)
	bolt.Speed = 0.75 * physics.ServerTickRateHz
	rules, err := predict.Load(patched(t, map[string]string{
		"projectile/tuning.json": fmt.Sprintf(`{"shots": {"bolt": {"speed": %v}}}`, bolt.Speed),
	}))
	if err != nil {
		t.Fatal(err)
	}
	step := bolt.Speed * physics.SecondsPerTick
	rewind := tuningOf[projectile.ProjectileTuning](t).RewindTicks()
	now := rewind + 4 // her state's tick: for her first frame the server keeps rows from tick 4
	// The enemy stands on her shot's path, or farther on it, or a tick later 2 units up off it, or dies
	// halfway through that tick there. A newer one can take its ID, never its serial.
	enemy := predict.Sight{ID: 7, Serial: 70, Row: flight.Row{Kind: interact.Body, Side: interact.Side{Owner: 7, Team: 1},
		Center: box2d.Vec2{X: 1}, Radius: 0.5, Until: 1}}
	far, up, dying, newer := enemy, enemy, enemy, enemy
	far.Center.X = 2
	farUp := far
	farUp.Center.Y = 3
	up.Center.Y = 2
	dying.Until = 0.5
	newer.Serial = 71
	diedUp := up
	diedUp.Until = 0.5
	sync := func(p *predict.Predictor, tick uint64, s predict.Sight) { p.See(tick, []predict.Sight{s}) }
	at := func(x, y float64) *box2d.Vec2 { return &box2d.Vec2{X: x, Y: y} }
	for _, c := range []struct {
		name  string
		see   func(p *predict.Predictor)
		view  float64
		drawn *box2d.Vec2 // where her screen drew the enemy, if her shot hits it
	}{
		{"seen", func(p *predict.Predictor) { sync(p, 10, enemy); sync(p, 11, enemy) }, 10.5, at(1, 0)},
		{"seen a tick's flight away", func(p *predict.Predictor) { sync(p, 10, far); sync(p, 11, far) }, 10.5, at(2, 0)},
		{"no sync of the older tick", func(p *predict.Predictor) { sync(p, 11, enemy); sync(p, 12, enemy) }, 10.5, nil},
		{"gone from the newest", func(p *predict.Predictor) {
			sync(p, 10, enemy)
			sync(p, 11, enemy)
			p.See(12, nil)
		}, 10.5, nil},
		{"drawn a quarter of the way up", func(p *predict.Predictor) { sync(p, 10, enemy); sync(p, 11, up) }, 10.25,
			at(1, 0.5)},
		{"drawn most of the way up", func(p *predict.Predictor) { sync(p, 10, enemy); sync(p, 11, up) }, 10.9, nil},
		// The server removes it at the end of tick 11, before her frames.
		{"dead before her frames, though drawn alive", func(p *predict.Predictor) { sync(p, 10, enemy); sync(p, 11, dying) },
			10.25, nil},
		// Judged at the oldest view the server keeps, 5, drawn from syncs 4 and 5: the predictor keeps both.
		{"drawn before the rows the server keeps", func(p *predict.Predictor) {
			for tick := range rewind + 1 {
				sync(p, 4+tick, enemy)
			}
		}, 1.5, at(1, 0)},
		// Each step of a replay is held at its own tick: her second is judged at 6, where the enemy stood on
		// her path; at 5 it was 3 units up.
		{"drawn before the rows the server keeps, a step later", func(p *predict.Predictor) {
			for tick := range rewind + 1 {
				sync(p, 4+tick, map[bool]predict.Sight{true: farUp, false: far}[tick < 2])
			}
		}, 1.5, at(2, 0)},
		{"drawn after them", func(p *predict.Predictor) {
			sync(p, now-2, far)
			sync(p, now-1, enemy)
			sync(p, now, enemy)
		}, float64(now) + 0.5, at(1, 0)},
		// As the server does, it draws the newer one where its first row is, not from the one it took the ID of.
		{"new with a dead one's ID", func(p *predict.Predictor) { sync(p, 10, diedUp); sync(p, 11, newer) }, 10.25,
			at(1, 0)},
	} {
		p := predict.New(rules)
		c.see(p)
		p.Reset(predict.State{Tick: now, Body: 1, Radius: 0.5, MoveSpeed: 5, Champion: "starter"})
		var ended []predict.Ended
		for i, buttons := range []uint32{1, 0} { // she casts, then holds
			_, stepEnded := p.Step(predict.Frame{Seq: uint32(i + 1), AimX: 1, Buttons: buttons, ViewTick: c.view})
			ended = append(ended, stepEnded...)
		}
		hits := c.drawn != nil
		if hit := len(ended) == 1 && ended[0].End.Hit.Target == enemy.ID; hit != hits || len(ended) > 1 {
			t.Errorf("%s: ended %+v, want a hit on %d: %v", c.name, ended, enemy.ID, hits)
			continue
		}
		if !hits {
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

var update = flag.Bool("update", false,
	"rewrite testdata from TestWireFixtures, TestPredictionMatchesServer and TestViewFixture")

// TestWireFixtures pins each payload's bytes in testdata/wire, which the client checks too
// (client/src/game/wire_fixtures.test.ts). A renamed field fails here instead of silently zeroing on
// the other side.
func TestWireFixtures(t *testing.T) {
	in := player.Input{Session: 305419896, Frames: cardinal.SliceOf(
		playercomp.InputFrame{Seq: 1, MoveX: 1},
		playercomp.InputFrame{
			Seq: 2, MoveX: 0.7071067811865476, MoveY: -0.7071067811865476, AimX: 3.5, AimY: -1.25, Buttons: 1,
			ViewTick: 41.375,
		},
	)}
	samples := []struct {
		name    string
		payload any
	}{
		{"body_sync", player.BodySync{
			Tick:        9007199254740993, // 2^53+1, past float64 precision
			TickSeconds: 0.05,
			SimVersion:  "go1.27.1+0123456789abcdef",
			You: player.YouState{
				Body: 1, InputSession: 4294967295, InputAck: 42, InputQueued: 3, InputCap: 10, MoveSpeed: 5,
				Champion: "starter",
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
					Type: "bolt", Ability: "bolt", Seq: 4294967295, Launch: 1, PrevX: -4.75, PrevY: 2.125, X: -4.25, Y: 1.5,
					VelX: 9, VelY: -12, Radius: 0.15, Remaining: 17.75,
				}),
				EchoDelay: 100,
				EchoPath:  cardinal.SliceOf(physics2d.Vec2{X: -3.5, Y: 0.25}, physics2d.Vec2{X: -3.25, Y: 0.5}),
			},
			Bodies: cardinal.SliceOf(
				player.BodyState{EntityID: 1, Serial: 1, PlayerID: 1, Nickname: "Player One", Team: 0, X: 1.5, Y: -2,
					Radius: 0.5},
				player.BodyState{EntityID: 2, Serial: 9007199254740993, PlayerID: 2, Nickname: "Échø ✦", Team: 1, X: -0.25,
					Y: 3.75, Radius: 0.75,
					Tags: interact.Stunned | interact.CC},
			),
		}},
		{"echo_sync", echo.EchoSync{Tick: 7, Echoes: cardinal.SliceOf(
			echo.EchoState{EntityID: 3, Serial: 3, PlayerID: 1, Owner: 1, Team: 0, X: -4.5, Y: 0.5, Radius: 0.5,
				Tags: interact.Unstoppable, HP: 62.5, MaxHP: 100, Until: 1},
			echo.EchoState{EntityID: 4, Serial: 4, PlayerID: 2, Owner: 2, Team: 1, X: 3, Y: -1.25, Radius: 0.5, MaxHP: 100,
				Until: 0.625},
		)}},
		{"ability_sync", ability.AbilitySync{Tick: 7, Abilities: cardinal.SliceOf(
			ability.AbilityState{Body: 1, Slot: 0, Ability: "bolt", ReadyTick: 18446744073709551615},
		)}},
		{"projectile_sync", projectile.ProjectileSync{Tick: 8, Projectiles: cardinal.SliceOf(
			projectile.ProjectileState{EntityID: 9, Serial: 18446744073709551615, Type: "bolt", Caster: 4294967295,
				Predicted: true, Team: 1, PrevX: -0.5, PrevY: -6.75, X: 0.125, Y: -7.5, Radius: 0.15, Until: 1},
			projectile.ProjectileState{EntityID: 10, Serial: 12, Type: "bolt", Caster: 3, Team: 0, PrevX: 2.25, PrevY: 0.5,
				X: 2.5, Y: 1, Radius: 0.15, Until: 0.375, Ended: true, OnTarget: true, TargetSerial: 4, OffsetX: -0.625,
				OffsetY: 0.125},
		)}},
		{"projectile_sync_empty", projectile.ProjectileSync{Tick: 9}},
		{"target_sync", projectile.TargetSync{Tick: 9007199254740993, Targets: cardinal.SliceOf(
			projectile.TargetState{EntityID: 0, Serial: 1, Kind: interact.Body, Owner: 0, Team: 0, X: 1.5, Y: -2, Radius: 0.5,
				Tags: interact.Stunned | interact.CC, Until: 1},
			projectile.TargetState{EntityID: 4, Serial: 18446744073709551615, Kind: interact.Echo, Owner: 2, Team: 1, X: 3,
				Y: -1.25, Radius: 0.5, Until: 0.625},
		)}},
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

// viewFixture is testdata/view.json: targets as a few ticks of syncs show them, and where views across
// those ticks draw each.
type viewFixture struct {
	Syncs []viewSync
	Views []viewDrawn
}

type viewSync struct {
	Tick    uint64
	Targets []viewTarget
}

type viewTarget struct {
	EntityID uint32
	Serial   uint64
	X, Y     float64
	Until    float64
}

type viewDrawn struct {
	View  float64
	Drawn []viewSpot
}

type viewSpot struct {
	Serial uint64
	X, Y   float64
}

// TestViewFixture pins where a view draws each target, which the client's screen draws by too
// (client/src/engine/render/entity_views.test.ts), to the bit: a frame's shots meet them there. A view
// draws the syncs at flight.RowTicks with flight.AtView, by serial, and nothing without both syncs, as
// the predictor places them.
func TestViewFixture(t *testing.T) {
	at := func(id uint32, serial uint64, x, y, until float64) viewTarget {
		return viewTarget{EntityID: id, Serial: serial, X: x, Y: y, Until: until}
	}
	// 1 walks; 2 is new at tick 12; 3 dies 0.375 into tick 12; 4 dies as tick 11 begins, so it's never
	// drawn, and a new target takes its ID at tick 12, drawn where it first is.
	fixture := viewFixture{Syncs: []viewSync{
		{10, []viewTarget{at(1, 11, 0, 0, 1), at(3, 13, -1, 2, 1), at(4, 14, 7, 7, 1)}},
		{11, []viewTarget{at(1, 11, 0.1, 0.7, 1), at(3, 13, -2.2, 2.5, 1), at(4, 14, 7, 7, 0)}},
		{12, []viewTarget{at(1, 11, 2.5, 1.3, 1), at(2, 12, 5, 5, 1), at(3, 13, -3, 4.1, 0.375), at(4, 15, -6, 1, 1)}},
		{13, []viewTarget{at(1, 11, 3, 3.3, 1), at(2, 12, 5.5, 4, 1), at(4, 15, -5.5, 1.5, 1)}},
	}}
	syncAt := func(tick uint64) (map[uint64]flight.Row, []viewTarget, bool) {
		for _, s := range fixture.Syncs {
			if s.Tick == tick {
				rows := map[uint64]flight.Row{}
				for _, x := range s.Targets {
					rows[x.Serial] = flight.Row{Tick: tick, Center: box2d.Vec2{X: x.X, Y: x.Y}, Until: x.Until}
				}
				return rows, s.Targets, true
			}
		}
		return nil, nil, false
	}
	// Before the first two syncs, through each tick's middle and end, at a death, and past the newest.
	for _, view := range []float64{9.75, 10, 10.3, 11, 11.2, 11.375, 11.5, 12, 12.7, 13, 13.25} {
		drawn := viewDrawn{View: view, Drawn: []viewSpot{}}
		a, b := flight.RowTicks(view)
		older, _, hasOlder := syncAt(a)
		newer, targets, _ := syncAt(b)
		if !hasOlder {
			targets = nil
		}
		for _, x := range targets {
			o, ok := older[x.Serial]
			if row, ok := flight.AtView(o, ok, newer[x.Serial], view); ok {
				drawn.Drawn = append(drawn.Drawn, viewSpot{Serial: x.Serial, X: row.Center.X, Y: row.Center.Y})
			}
		}
		fixture.Views = append(fixture.Views, drawn)
	}
	got, err := json.MarshalIndent(fixture, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "view.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if want, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s no longer matches how a view draws (%v). If intended, run "+
			"`go test ./shards/game -run TestViewFixture -update`, then make the client's draw match", path, err)
	}
}
