package main

import (
	"context"
	"crypto/sha256"
	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/ability/hooks"
	"echo_image/shards/game/internal/area"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/effect"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/projectile"
	"echo_image/shards/game/internal/projectile/flight"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io/fs"
	"math"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
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

// touching reports whether shot s ended touching a circle of radius r at (x, y). Box2D stops a cast a
// slop short.
func touching(s projectile.ProjectileState, x, y, r float64) bool {
	return math.Abs(math.Hypot(s.X-x, s.Y-y)-(s.Radius+r)) <= 2*box2d.LinearSlop && s.Until > 0 && s.Until <= 1
}

// flyFor ticks n times with no input.
func flyFor(w *cardinal.TestWorld, n int) {
	for range n {
		w.Tick()
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

var (
	update = flag.Bool("update", false,
		"rewrite testdata from TestWireFixtures, TestPredictionMatchesServer and TestViewFixture")
	recordTo = flag.String("record", "", "write TestPredictionMatchesServer's run to this file, for the wasm build")
)
