package test

import (
	"context"
	"echo_image/pkg/level"
	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/ability/hooks"
	"echo_image/shards/game/internal/arena"
	"echo_image/shards/game/internal/echo"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/history"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/physics/timestep"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/projectile"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/shard"
	"encoding/json"
	"flag"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func input(session uint32, frames ...playercomp.InputFrame) player.Input {
	return player.Input{Session: session, Frames: cardinal.SliceOf(frames...)}
}

func frame(seq uint32, x, y float64) playercomp.InputFrame {
	return playercomp.InputFrame{Seq: seq, MoveX: x, MoveY: y}
}

// tuningOf reads the shipped tuning for T as the shard loads it, so tests follow the real numbers.
func tuningOf[T content.File](t *testing.T) T {
	t.Helper()
	v, err := content.Load[T](content.FS)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// withContent is Register with each patch deep-merged into the shipped content file it names, in
// order, so a test can add a shot type or a champion. Objects merge, and anything else replaces.
func withContent(t testing.TB, patches ...map[string]string) func(*cardinal.World) {
	t.Helper()
	return withFiles(patched(t, patches...))
}

// withFiles is Register with files in place of the shipped content.
func withFiles(files content.Overlay) func(*cardinal.World) {
	return func(world *cardinal.World) { shard.RegisterWith(world, files, otel.GetMeterProvider()) }
}

// patched is the content withContent registers, which a predictor can load too.
func patched(t testing.TB, patches ...map[string]string) content.Overlay {
	t.Helper()
	docs := map[string]map[string]any{}
	for _, files := range patches {
		for file, patch := range files {
			if docs[file] == nil {
				raw, err := content.FS.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				var doc map[string]any
				if err := json.Unmarshal(raw, &doc); err != nil {
					t.Fatal(err)
				}
				docs[file] = doc
			}
			var add map[string]any
			if err := json.Unmarshal([]byte(patch), &add); err != nil {
				t.Fatalf("patch for %s: %v", file, err)
			}
			merge(docs[file], add)
		}
	}
	files := content.Overlay{}
	for file, doc := range docs {
		var err error
		if files[file], err = json.Marshal(doc); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// deepWindow lets a shot's view go back 0.4 s (test content), for tests that draw bob from well before
// he moved.
var deepWindow = map[string]string{"history/tuning.json": `{"window": {"interpolation_seconds": 0.1,
	"queue_seconds": 0.05, "ping_seconds": 0.25}}`}

// effectKinds are the effect kinds tests put on things through an emitter. No ability uses them yet, so
// they stay out of the shipped content.
var effectKinds = map[string]string{"effect/tuning.json": `{"kinds": {
	"phase": {"passes": ["wall"], "stacking": "merge"},
	"stun":  {"tags": ["stunned", "cc"], "ends": ["ends_on_cc"], "blocked_by": ["unstoppable"], "stacking": "merge"},
	"slow":  {"tags": ["cc"], "stat": "move_speed", "blocked_by": ["unstoppable"], "stacking": "per_source"}}}`}

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

// body is a player's own Body as its body_sync shows it: what its row lacks, and its row from You.
type body struct {
	player.BodyState
	Team   uint8
	X, Y   float64
	Radius float64
}

// yourBody returns the receiving player's own Body.
func yourBody(t *testing.T, s player.BodySync) body {
	t.Helper()
	for _, b := range s.Bodies.All() {
		if b.EntityID == s.You.Body {
			return body{BodyState: b, Team: s.You.Team, X: s.You.X, Y: s.You.Y, Radius: s.You.Radius}
		}
	}
	t.Fatalf("your Body %d is not in the state", s.You.Body)
	return body{}
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
func ticks(seconds float64) int { return timestep.Ticks(seconds) }

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
	w = cardinal.NewTestWorld(t, shard.Register)
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

// join has player take the next free Body in w, standing still. The dev roster's order sets its team.
func join(t *testing.T, w *cardinal.TestWorld, player string) *client {
	t.Helper()
	c := &client{w: w, id: player, session: 1, back: drawLag}
	c.move(0, 0)
	w.Tick()
	yourBody(t, bodiesFor(t, w, player))
	return c
}

// twoVsTwo is a 2v2 (test content), with patches on top: a dev roster of four, on a map where carol
// starts on alice's team 2 m in front of her, and dave on bob's 2 m in front of him. After claim, carol
// and dave join.
func twoVsTwo(t testing.TB, patches ...map[string]string) content.Overlay {
	t.Helper()
	roster := map[string]string{"player/tuning.json": `{"dev_spawns": [{"team": 0}, {"team": 1}, {"team": 0},
		{"team": 1}]}`}
	return onMap(t, patched(t, append([]map[string]string{roster}, patches...)...), level.Map{Objects: []level.Object{
		spawnAt(0, -5, 0), spawnAt(0, -3, 0), spawnAt(1, 5, 0), spawnAt(1, 3, 0),
	}})
}

// skirmish is a 3v3 that fights on (test content: no Echo dies). Each step, every player walks up or
// down and holds fire at the other side.
func skirmish(tb testing.TB) (step func()) {
	tb.Helper()
	files := patched(tb, map[string]string{
		"player/tuning.json": `{"dev_spawns": [{"team": 0}, {"team": 1}, {"team": 0}, {"team": 1}, {"team": 0},
			{"team": 1}]}`,
		"echo/tuning.json": `{"max_hp": 1e9}`,
	})
	w := cardinal.NewTestWorld(tb, withFiles(onMap(tb, files, level.Map{Objects: []level.Object{
		spawnAt(0, -5, 0), spawnAt(0, -5, 3), spawnAt(0, -5, -3), spawnAt(1, 5, 0), spawnAt(1, 5, 3), spawnAt(1, 5, -3),
	}})))
	var players []*client
	for _, id := range []string{"a1", "b1", "a2", "b2", "a3", "b3"} {
		players = append(players, &client{w: w, id: id, session: 1, back: drawLag})
		players[len(players)-1].move(0, 0)
	}
	w.Tick()
	n := 0
	return func() {
		dy := float64(1 - 2*(n/ticks(1)%2))
		for i, p := range players {
			p.send(playercomp.InputFrame{MoveY: dy, AimX: float64(1 - 2*(i%2)), Buttons: 1})
		}
		w.Tick()
		n++
	}
}

// BenchmarkTick is a 3v3 tick at this build's rate (-tags rate60 for 60 Hz).
func BenchmarkTick(b *testing.B) {
	step := skirmish(b)
	for range ticks(5) { // shots in flight, Echoes on their trails
		step()
	}
	b.ReportAllocs()
	for b.Loop() {
		step()
	}
}

// A 3v3 tick allocates no more than its ceiling, so a rise in cost shows before it ships. Allocation
// counts, unlike times, don't depend on the machine. Raise a ceiling only for a known reason.
func TestTickAllocations(t *testing.T) {
	step := skirmish(t)
	for range ticks(5) {
		step()
	}
	ceiling := map[int]float64{20: 4000, 60: 6000}[physics.ServerTickRateHz] // measured 3,300 and 4,976
	if got := testing.AllocsPerRun(ticks(2), step); got > ceiling {
		t.Fatalf("a 3v3 tick allocates %.0f times, past its ceiling of %.0f", got, ceiling)
	}
}

// echoOf returns the Echo of the player who owns the Body in s, as viewer sees it.
func echoOf(t *testing.T, w *cardinal.TestWorld, viewer string, owner body) echoSeen {
	t.Helper()
	for _, e := range sentTo[echo.EchoSync](t, w, viewer).Echoes.All() {
		if e.PlayerID == owner.PlayerID {
			r := rowOf(t, w, viewer, e.Serial)
			return echoSeen{EchoState: e, Owner: r.Owner, Team: r.Team, X: r.X, Y: r.Y, Radius: r.Radius, Tags: r.Tags,
				Until: r.Until}
		}
	}
	t.Fatalf("no Echo for player %d", owner.PlayerID)
	return echoSeen{}
}

// rowOf is the row viewer's target_sync shows of the Body or Echo with serial.
func rowOf(t *testing.T, w *cardinal.TestWorld, viewer string, serial uint64) history.TargetState {
	t.Helper()
	for r := range sentTo[history.TargetSync](t, w, viewer).Targets.Values() {
		if r.Serial == serial {
			return r
		}
	}
	t.Fatalf("%s's target_sync has no row of serial %d", viewer, serial)
	return history.TargetState{}
}

// echoSeen is an Echo as a viewer's syncs show it: what its row lacks, and its row.
type echoSeen struct {
	echo.EchoState
	Owner  uint32
	Team   uint8
	X, Y   float64
	Radius float64
	Tags   interact.Tags
	Until  float64
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

// Content fails the boot when a key is unknown (a typo would load as 0) or an ID one file names is
// missing from another, so every boot, test or shard, checks the content it runs.
func TestBadContentFailsTheBoot(t *testing.T) {
	for _, c := range []struct{ name, file, patch, want string }{
		{"typo key", "ability/tuning.json", `{"abilities": {"bolt": {"cooldown_secnds": 2}}}`, "cooldown_secnds"},
		{"typo nested key", "area/tuning.json", `{"kinds": {"anticamp": {"rules": [{"damage_per_secnd": 9}]}}}`,
			"damage_per_secnd"},
		{"champion without an Echo", "player/tuning.json",
			`{"champions": {"rook": {"move_speed": 5, "radius": 0.5}}}`, `champion "rook" has no Echo`},
		{"Echo of no champion", "echo/tuning.json", `{"champions": {"rook": {"delay_seconds": 2}}}`,
			`Echo "rook" belongs to no champion`},
		{"unknown Echo area", "echo/tuning.json", `{"champions": {"starter": {"area": "aera"}}}`,
			`unknown area kind "aera"`},
		{"unknown death effect", "echo/tuning.json", `{"death_effect": "dead"}`, `unknown effect kind "dead"`},
		{"unknown shot", "ability/tuning.json", `{"abilities": {"bolt": {"params": {"shot": "blot"}}}}`,
			`unknown shot "blot"`},
		{"unknown dev champion", "player/tuning.json", `{"dev_champion": "rook"}`, `dev_champion "rook"`},
		{"window that favors no ping", "history/tuning.json", `{"window": {"ping_seconds": 0}}`, "ping_seconds"},
		{"payload with no relations", "projectile/tuning.json",
			`{"shots": {"bolt": {"meets": [{"kinds": ["echo"], "do": "stop", "payload": true}]}}}`, "names no relations"},
		{"rule for shots meeting shots", "projectile/tuning.json",
			`{"shots": {"bolt": {"meets": [{"kinds": ["shot"], "do": "stop"}]}}}`, "names a kind a shot never meets"},
		{"default map not listed", "arena/tuning.json", `{"default_map": "cave"}`, `default map "cave"`},
		{"listed map with no file", "arena/tuning.json", `{"maps": ["open", "cave"]}`, "cave.level.json"},
		{"map that doesn't decode", "arena/open.level.json", `{"format": 2}`, "map open: level: format 2"},
		{"map short of spawns", "player/tuning.json", `{"dev_spawns": [{"team": 0}, {"team": 0}]}`,
			"map open: arena: team 0 has spawns for 1 of its 2 players"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := shard.CheckContent(patched(t, map[string]string{c.file: c.patch}))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("loaded with error %v, want one naming %s", err, c.want)
			}
		})
	}
	if err := shard.CheckContent(content.FS); err != nil {
		t.Fatalf("the shipped content fails: %v", err)
	}
}

// ran is a system a tick ran: its type's name and its hook.
type ran struct{ name, hook string }

// systemsRun returns the systems one tick of the shard runs, in order. Read from the tick's spans, so it
// is the order that ran, wherever each system was registered.
func systemsRun(t *testing.T) []ran {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	traced := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	before := otel.GetTracerProvider()
	otel.SetTracerProvider(traced)
	t.Cleanup(func() {
		otel.SetTracerProvider(before)
		_ = traced.Shutdown(context.Background()) // before forwards to traced for good, so turn it off
	})
	cardinal.NewTestWorld(t, shard.Register).Tick()

	var systems []ran
	for _, span := range spans.Ended() {
		var s ran
		for _, kv := range span.Attributes() {
			switch kv.Key {
			case "cardinal.system.name":
				s.name = strings.TrimPrefix(kv.Value.AsString(), "*")
			case "cardinal.system.hook":
				s.hook = kv.Value.AsString()
			}
		}
		if s.name != "" && s.hook != "SYSTEM_HOOK_INIT" {
			systems = append(systems, s)
		}
	}
	return systems
}

// The map a tick's state names is live before any system reads it: levelstate's system runs first.
func TestMapIsLiveFirstInTheTick(t *testing.T) {
	systems := systemsRun(t)
	if len(systems) == 0 || !strings.HasPrefix(systems[0].name, "levelstate.") {
		t.Fatalf("a tick runs %v, want a levelstate system first", systems)
	}
}

// Every recorder and sync reads the tick's final state: each runs in PostUpdate, after every writer,
// from history.RecordSystem to combat.DeathSystem, the one system there that changes the world.
func TestReadersRunInPostUpdate(t *testing.T) {
	reader := func(name string) bool {
		return strings.HasSuffix(name, ".RecordSystem") || strings.HasSuffix(name, ".SyncSystem")
	}
	var post []string
	for _, s := range systemsRun(t) {
		switch {
		case s.hook == "SYSTEM_HOOK_POST_UPDATE":
			post = append(post, s.name)
		case reader(s.name) || s.name == "combat.DeathSystem":
			t.Errorf("%s runs before PostUpdate", s.name)
		}
	}
	if len(post) < 2 || post[0] != "history.RecordSystem" || post[len(post)-1] != "combat.DeathSystem" {
		t.Fatalf("PostUpdate runs %v, want history.RecordSystem first and combat.DeathSystem last", post)
	}
	for _, name := range post[1 : len(post)-1] {
		if !reader(name) {
			t.Errorf("%s runs in PostUpdate, where only recorders and syncs come before DeathSystem", name)
		}
	}
}

// spawnOf is where the dev roster's Body i starts on the shipped default map, and its radius, known
// before boot so a test can put its map's walls around it. Players claim Bodies in roster order: alice's
// is 0, bob's 1.
func spawnOf(t *testing.T, i int) (box2d.Vec2, float64) {
	t.Helper()
	tuning := tuningOf[player.MovementTuning](t)
	team, before := tuning.DevSpawns[i].Team, 0 // Bodies of its team before it
	for _, d := range tuning.DevSpawns[:i] {
		if d.Team == team {
			before++
		}
	}
	return arena.Spawns(shippedMap(t))[team][before], tuning.Champions[tuning.DevChampion].Radius
}

// shippedMap is the shipped default map.
func shippedMap(t *testing.T) level.Map {
	t.Helper()
	tuning := tuningOf[arena.ArenaTuning](t)
	files, err := tuning.Files(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	m, err := level.Decode(files[tuning.DefaultMap], arena.Schema())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// shippedSpawns are the shipped default map's spawns, for a test map where Bodies start as they do there.
func shippedSpawns(t *testing.T) []level.Object {
	t.Helper()
	var spawns []level.Object
	for _, o := range shippedMap(t).Objects {
		if _, ok := o.Props.(arena.Spawn); ok {
			spawns = append(spawns, o)
		}
	}
	return spawns
}

// spawnAt is a spawn of team at (x, y), for a test map.
func spawnAt(team uint8, x, y float64) level.Object {
	return level.Object{Type: "spawn", At: mapPoint(x, y), Props: arena.Spawn{Team: team}}
}

// wall is a box wall that blocks Bodies and shots, for a test map.
func wall(center, half box2d.Vec2) level.Solid { return mapBox("wall", center, half) }

// mapBox is a box of a map kind (wall, or bounds: the map edge), for a test map. Like every map shape it
// masks all bits, so each query's mask decides whether it stops there. Its corners round to the map
// file's grid.
func mapBox(kind string, center, half box2d.Vec2) level.Solid {
	var corners []box2d.Vec2
	for _, c := range [][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		p := mapPoint(center.X+c[0]*half.X, center.Y+c[1]*half.Y)
		corners = append(corners, p.Vec2())
	}
	hull := box2d.ComputeHull(corners) // a file holds a polygon in its hull's order
	points := make([]level.Point, hull.Count)
	for i := range points {
		points[i] = mapPoint(hull.Points[i].X, hull.Points[i].Y)
	}
	return level.Solid{Kind: kind, Polygon: points}
}

// mapPoint is (x, y) in a map file's units, rounded.
func mapPoint(x, y float64) level.Point {
	return level.Point{int32(math.Round(x * level.Unit)), int32(math.Round(y * level.Unit))}
}

// mapLoop is a loop of a map kind through points (x, y), for a test map: an outline winds
// counter-clockwise, an edge clockwise.
func mapLoop(kind string, points [][2]float64) level.Loop {
	l := level.Loop{Kind: kind}
	for _, p := range points {
		l.Points = append(l.Points, mapPoint(p[0], p[1]))
	}
	return l
}

// onMap is files with m, a test map, live at boot in place of the shipped maps.
func onMap(t testing.TB, files content.Overlay, m level.Map) content.Overlay {
	t.Helper()
	m.Solids, m.Outlines, m.Edges = slices.Clone(m.Solids), slices.Clone(m.Outlines), slices.Clone(m.Edges)
	m.Objects = slices.Clone(m.Objects)
	src := uint32(0)
	for i := range m.Solids {
		src++
		m.Solids[i].Src = src
	}
	for _, loops := range [][]level.Loop{m.Outlines, m.Edges} {
		for i := range loops {
			src++
			loops[i].Src = src
		}
	}
	for i := range m.Objects {
		src++
		m.Objects[i].ID = src
	}
	data, err := level.Encode(m, arena.Schema())
	if err != nil {
		t.Fatal(err)
	}
	files = maps.Clone(files)
	files["arena/tuning.json"] = []byte(`{"maps": ["test"], "default_map": "test"}`)
	files["arena/test.level.json"] = data
	return files
}

// duelOn is duel on a test map of solids, with the shipped map's spawns.
func duelOn(t *testing.T, solids ...level.Solid) (w *cardinal.TestWorld, alice, bob *client) {
	t.Helper()
	w = cardinal.NewTestWorld(t, withFiles(onMap(t, content.Overlay{}, level.Map{Solids: solids,
		Objects: shippedSpawns(t)})))
	alice, bob = claim(t, w)
	return w, alice, bob
}

// fxDuelOn is fxDuel on a test map of solids, with the shipped map's spawns.
func fxDuelOn(t *testing.T, solids ...level.Solid) (w *cardinal.TestWorld, alice, bob *client, fx *emitter) {
	t.Helper()
	fx = &emitter{}
	files := onMap(t, patched(t, effectKinds), level.Map{Solids: solids, Objects: shippedSpawns(t)})
	w = cardinal.NewTestWorld(t, withEmitter(withFiles(files), fx))
	alice, bob = claim(t, w)
	return w, alice, bob, fx
}

// radiusOf returns the radius of b's collider circle.
func radiusOf(w *cardinal.TestWorld, b body) float64 {
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
func (e *emitter) effect(b body, in effectcomp.EffectIntent) {
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
	w = cardinal.NewTestWorld(t, withEmitter(withContent(t, effectKinds), fx))
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
	seed     = flag.Uint64("seed", 0, "TestRandomPlayIsPredicted's seed, to rerun a failure; 0 picks one")
)
