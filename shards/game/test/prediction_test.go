package test

import (
	"cmp"
	echoimage "echo_image"
	"echo_image/pkg/level"
	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability/cast"
	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/history"
	"echo_image/shards/game/internal/history/past"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/predict"
	projectilecomp "echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/shard"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/vmihailenco/msgpack/v5"
)

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
				e, ok := ownEcho(sentTo[history.TargetSync](t, w, "alice"), bodiesFor(t, w, "alice").You.Body)
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

// Your screen draws your Echo where your predictor has it, as it does your Body: across a frame, from
// where it was a tick ago to where it is now. So a shot that stops on it meets it there, on the server
// and in your predictor alike (test content: a bolt that stops on its caster's Echo). Her Echo walks
// toward her along her shot's path, and her view had it 0.125 s further back. Her bolt ends touching it
// where it was when they met.
func TestShotMeetsYourEchoWhereYouDrawIt(t *testing.T) {
	patch := map[string]string{
		"echo/tuning.json": `{"champions": {"starter": {"delay_seconds": 1}}}`,
		"projectile/tuning.json": `{"shots": {"bolt": {"meets": [
			{"kinds": ["echo"], "relations": ["own"], "do": "stop"}]}}}`,
	}
	w := cardinal.NewTestWorld(t, withContent(t, patch))
	alice, _ := claim(t, w)
	_, bolt := primary(t)
	moveFor(w, alice, 1, 0, physics.ServerTickRateHz) // her Echo walks this a second later
	flyFor(w, ticks(0.25))
	rules, err := predict.Load(patched(t, patch))
	if err != nil {
		t.Fatal(err)
	}
	p := predict.New(rules)
	body := yourBody(t, bodiesFor(t, w, "alice")).EntityID
	var ended []predict.Ended
	var was history.TargetState // her Echo a tick before
	for i := 0; len(ended) == 0; i++ {
		if i == ticks(0.5) {
			t.Fatal("her bolt never met her Echo")
		}
		before := bodiesFor(t, w, "alice")
		see(t, p, sentTo[history.TargetSync](t, w, "alice"))
		was, _ = ownEcho(sentTo[history.TargetSync](t, w, "alice"), body)
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
	e, _ := ownEcho(sentTo[history.TargetSync](t, w, "alice"), body)
	end := ended[0].End
	met := box2d.Vec2{X: was.X + (e.X-was.X)*end.Until, Y: was.Y + (e.Y-was.Y)*end.Until}
	if end.Hit.Target != e.EntityID || math.Abs(end.At.X-end.Hit.Offset.X-met.X) > 1e-9 ||
		math.Abs(end.At.Y-end.Hit.Offset.Y-met.Y) > 1e-9 || !(end.Until > 0) ||
		math.Abs(box2d.Length(end.Hit.Offset)-(bolt.Radius+e.Radius)) > 2*box2d.LinearSlop {
		t.Fatalf("her bolt ended %+v, want touching her Echo where it was when they met, %v", ended[0], met)
	}
}

// body_sync says which sim the shard runs, so a client predicts only with a predictor of the same
// (predictor.mjs stamps its wasm from the same function: predictor.test.ts).
func TestBodySyncSaysWhichSimItRuns(t *testing.T) {
	w := cardinal.NewTestWorld(t, shard.Register)
	w.Command("alice", input(1, frame(1, 0, 0)))
	w.Tick()
	if got, want := bodiesFor(t, w, "alice").SimVersion, echoimage.SimVersion(); got != want {
		t.Fatalf("body_sync says sim %q, want %q", got, want)
	}
}

// predictionStep is one tick of TestPredictionMatchesServer's run: the body_sync and target_sync before
// it, the frames her client stepped from them (the one the server applied, or on the bad network every
// one unacked, maybe none), and what the predictor gave after, as predictor.wasm returns it.
type predictionStep struct {
	Sync    player.BodySync
	Targets history.TargetSync
	Frames  []playercomp.InputFrame
	X, Y    float64          // your Body
	Shots   []shotPrediction // your shots, in the predictor's order
	Ended   []endedPrediction
	Kit     kitPrediction
	Echo    *echoPrediction // nil with no Echo
}

// recording is what -record writes: the run, and the content files it played in place of the shipped
// ones, so the wasm build plays them too.
type recording struct {
	Content map[string]string
	Steps   []predictionStep
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

// at is line i of lines, or "(none)".
func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "(none)"
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
	you := got.You
	state := predict.State{
		Tick: got.Tick, Map: got.MapID, Body: you.Body, Team: you.Team, Center: box2d.Vec2{X: you.X, Y: you.Y},
		Radius: you.Radius, MoveSpeed: you.MoveSpeed, Champion: you.Champion,
		Cooldowns: slices.Collect(you.Cooldowns.Values()), Effects: slices.Collect(you.Effects.Values()),
		EffectsNext: you.EffectsNext, EchoDelay: you.EchoDelay, EchoSerial: you.EchoSerial,
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
func see(t *testing.T, p *predict.Predictor, targets history.TargetSync) {
	t.Helper()
	var sights []predict.Sight
	for s := range overWire(t, targets).Targets.Values() {
		sights = append(sights, predict.Sight{ID: s.EntityID, Serial: s.Serial, Row: past.Row{
			Kind: s.Kind, Side: interact.Side{Owner: s.Owner, Team: s.Team}, Center: box2d.Vec2{X: s.X, Y: s.Y},
			Radius: s.Radius, Tags: s.Tags, Until: s.Until,
		}})
	}
	p.See(targets.Tick, sights)
}

// ownEcho is the row of Body body's Echo in targets.
func ownEcho(targets history.TargetSync, body uint32) (history.TargetState, bool) {
	for r := range targets.Targets.Values() {
		if r.Kind == interact.Echo && r.Owner == body {
			return r, true
		}
	}
	return history.TargetState{}, false
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

// touched returns the shapes of a map kind in world that a circle at center touches: with radius 0, the
// ones the point is inside.
func touched(world *box2d.World, center box2d.Vec2, radius float64, kind interact.Kind) map[box2d.ShapeID]bool {
	proxy := box2d.MakeProxy([]box2d.Vec2{center}, 1, radius)
	shapes := map[box2d.ShapeID]bool{}
	world.OverlapShape(&proxy, box2d.QueryFilter{CategoryBits: uint64(interact.Body), MaskBits: uint64(kind)},
		func(s box2d.ShapeID, _ any) bool {
			shapes[s] = true
			return true
		}, nil)
	return shapes
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
func requirePredicted(t *testing.T, at string, g guess, after player.BodySync, targets history.TargetSync,
	effects bool,
) {
	t.Helper()
	got, want := g.state, predictFrom(t, after)
	if got.Center != want.Center {
		t.Fatalf("%s: predicted her Body at %v, the server has %v", at, got.Center, want.Center)
	}
	if e, found := ownEcho(targets, after.You.Body); g.hasEcho != found || found && g.echo != (box2d.Vec2{X: e.X, Y: e.Y}) {
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
func record(p *predict.Predictor, sync player.BodySync, targets history.TargetSync, frames []playercomp.InputFrame,
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

// recorded is the content TestPredictionMatchesServer's run plays: every number it reads, pinned, so a
// balance change never moves the recording. Her Echo trails her by a second.
// The tick your Echo dies, body_sync sends no path for it, so your predictor doesn't draw it the tick
// after, when the server has none.
func TestDeadEchoIsNotPredicted(t *testing.T) {
	w, alice, _, fx := fxDuel(t)
	rules, err := predict.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	mine := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice")))
	emit(fx, combatcomp.DamageIntent{Target: cardinal.EntityID(mine.EntityID), Amount: mine.MaxHP})
	w.Tick() // it dies, and is sent once more
	p := predict.New(rules)
	p.Reset(predictFrom(t, bodiesFor(t, w, "alice")))
	f := alice.send(playercomp.InputFrame{MoveX: 1})
	w.Tick()
	p.Step(predictFrame(f))
	requirePredicted(t, "the tick after", guessOf(p), bodiesFor(t, w, "alice"), sentTo[history.TargetSync](t, w, "alice"),
		true)
}

// Random but honest play: each player holds a random move for a while, closes in when far, aims near the
// other and presses random buttons. Her predictor, reset when a tick's state disagrees with it (Agrees),
// lands on the server's next one to the bit, as in TestPredictionMatchesServer. The only surprise is her Echo dying, which puts her
// out. A failure names its seed, and -seed reruns it.
func TestRandomPlayIsPredicted(t *testing.T) {
	s := *seed
	if s == 0 {
		s = rand.Uint64()
	}
	rng := rand.New(rand.NewPCG(s, 0))
	w, alice, bob := duel(t)
	rules, err := predict.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	p := predict.New(rules)
	me := yourBody(t, bodiesFor(t, w, "alice")).EntityID
	moves := map[*client][2]float64{}
	frame := func(c *client, from, to body) playercomp.InputFrame {
		if rng.IntN(ticks(0.5)) == 0 {
			moves[c] = [2]float64{rng.Float64()*2 - 1, rng.Float64()*2 - 1}
		}
		m, dx, dy := moves[c], to.X-from.X, to.Y-from.Y
		if dx*dx+dy*dy > 64 { // over 8 m apart
			m = [2]float64{dx, dy}
		}
		return playercomp.InputFrame{MoveX: m[0], MoveY: m[1], AimX: dx + rng.NormFloat64(), AimY: dy + rng.NormFloat64(),
			Buttons: uint32(rng.IntN(4))}
	}
	for i := range 2000 {
		before, targets := bodiesFor(t, w, "alice"), sentTo[history.TargetSync](t, w, "alice")
		mine, mineOK := ownEcho(sentTo[history.TargetSync](t, w, "alice"), me)
		a, b := yourBody(t, before), yourBody(t, bodiesFor(t, w, "bob"))
		see(t, p, targets)
		if s := predictFrom(t, before); !p.Agrees(before.You.InputAck, s) {
			p.Reset(s)
		}
		f := alice.next(frame(alice, a, b))
		_, ended := p.Step(predictFrame(f))
		alice.send(f)
		bob.send(frame(bob, b, a))
		w.Tick()
		died := mineOK && slices.ContainsFunc(w.Emitted[combatcomp.Died](), func(d combatcomp.Died) bool {
			return uint32(d.Target) == mine.EntityID
		})
		at := fmt.Sprintf("seed %d, tick %d", s, i)
		requirePredicted(t, at, guessOf(p), bodiesFor(t, w, "alice"), sentTo[history.TargetSync](t, w, "alice"), !died)
		requireEnded(t, at, ended, w.Emitted[projectilecomp.ShotEnded](), me)
	}
}

var recorded = map[string]string{
	"player/tuning.json": `{"input_buffer_seconds": 0.5, "input_timeout_seconds": 0.5, "echo_path_seconds": 0.5,
		"champions": {"starter": {"move_speed": 5.0, "radius": 0.5}}}`,
	"echo/tuning.json": `{"max_hp": 100, "hit_radius": 0.5, "death_effect": "out",
		"champions": {"starter": {"delay_seconds": 1, "area": "anticamp"}}}`,
	"area/tuning.json": `{"kinds": {"anticamp": {"rules": [
		{"kinds": ["body"], "relations": ["own"], "to": "self", "damage_per_second": 25}]}}}`,
	"ability/tuning.json": `{"blocked_by": ["stunned", "out"], "kits": {"starter": ["bolt", "dash"]}, "abilities": {
		"bolt": {"cooldown_seconds": 2.0, "aimed": true, "hook": "launch", "params": {"shot": "bolt"}},
		"dash": {"cooldown_seconds": 6.0, "aimed": true, "hook": "dash",
			"params": {"distance": 6.0, "seconds": 0.3, "effect": "dash"}}}}`,
	"projectile/tuning.json": `{"shots": {"bolt": {"speed": 15.0, "range": 20.0, "radius": 0.15, "damage": 25}}}`,
	"history/tuning.json":    `{"window": {"interpolation_seconds": 0.125, "queue_seconds": 0.05, "ping_seconds": 0.225}}`,
	"effect/tuning.json": `{"kinds": {
		"dash": {"tags": ["ends_on_cc"], "stacking": "merge"},
		"stun": {"tags": ["stunned", "cc"], "ends": ["ends_on_cc"], "blocked_by": ["unstoppable"], "stacking": "merge"},
		"slow": {"tags": ["cc"], "stat": "move_speed", "blocked_by": ["unstoppable"], "stacking": "per_source"},
		"phase": {"passes": ["wall"], "stacking": "merge"},
		"out": {"tags": ["out"], "stacking": "merge"}}}`,
}

// recordedMap is the map TestPredictionMatchesServer's run plays, around the dev spawns: her Body starts
// at (-5, 0), bob's at (5, 0). Wall 1 is under her, a solid, and wall 2 an outline up and to her left. The
// edge's top has joints, and its left side is past a bolt's range from her.
var recordedMap = level.Map{
	Solids:   []level.Solid{wall(box2d.Vec2{X: -4.75, Y: -3.9375}, box2d.Vec2{X: 3.25, Y: 3.0625})},
	Outlines: []level.Loop{mapLoop("wall", [][2]float64{{-10, 2.5}, {-8, 2.5}, {-8, 5}, {-10, 5}})},
	Edges: []level.Loop{mapLoop("bounds", [][2]float64{{-25, 6}, {-4, 6}, {-2, 6}, {0, 6}, {2, 6}, {4, 6}, {7, 6},
		{7, -15}, {-25, -15}})},
}

// The client predicts its Body with internal/predict, from what body_sync sends: its moves, its casts
// and their cooldowns, its dash, its shots and its Echo, through the walls of the map it names. Started
// from any tick's state and stepped by the frames after it, it lands on the server's bits, through slows,
// stuns, a phase, forced moves, held buttons, bad frames, walls, the map edge and a shot flying out its
// range. Her dash down crosses into wall 1 under bob's phase, his stun ends it there, and she lands on top
// when the phase runs out. Her first bolt ends on wall 2. Her second bolt meets bob, walking across it,
// where her screen drew him: the server ends it the same tick at the same point, and a replay of the frame
// ends it there too. Effects others put on her surprise it, and come with the next state. Her Echo trails
// her by a second (test content), so it retraces her path in the run. Bob walks up to the edge and stops
// on his Echo, its anti-camp kills it, and he is out: her last bolt flies through him to the edge, which
// she slides along on the bad network.
// testdata/prediction-20hz.txt (one per tick rate) is the digest of the run and the bad network's replays.
// -record writes the run, so the client checks its wasm build lands there too (predictor.test.ts).
func TestPredictionMatchesServer(t *testing.T) {
	fx := &emitter{}
	files := onMap(t, patched(t, recorded), recordedMap)
	w := cardinal.NewTestWorld(t, withEmitter(withFiles(files), fx))
	alice, bob := claim(t, w)
	rules, err := predict.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	p := predict.New(rules)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	from := yourBody(t, bodiesFor(t, w, "bob")).EntityID
	const dashSlot = 1 // recorded's kit: bolt, then dash
	boltCast, bolt := rules.Abilities.Abilities["bolt"], rules.Shots.Shots["bolt"]
	second := int(boltCast.CooldownTicks()) // the step her bolt is ready again
	step := bolt.Speed * physics.SecondsPerTick
	if steps := int(bolt.Range / step); steps >= second {
		t.Fatalf("a bolt flies %d ticks, past its cooldown", steps)
	}
	// What bob puts on her, by step. Each applies from the tick after it.
	surprises := map[int]effectcomp.EffectIntent{
		2: {Kind: "slow", Ticks: 9, Value: -4000, Source: from},
		3: {Kind: "phase", Ticks: 5, Source: from}, // runs out during the stun
		6: {Kind: "stun", Ticks: 3, Source: from},  // ends her dash
	}
	moves := [][2]float64{{1, 0}, {0.3, -0.9}, {-1, 1}, {0, 0}, {math.NaN(), 1}, {-0.6, -0.8}, {3, 4}, {1e-12, 0}}
	// She holds fire for six ticks, aiming up and left, away from bob: it casts once, then cools down.
	// She also dashes down on the fourth. Once her bolt is ready, she holds fire again, leading bob.
	var lead box2d.Vec2
	frameAt := func(i int) playercomp.InputFrame {
		m := moves[i%len(moves)]
		f := playercomp.InputFrame{MoveX: m[0], MoveY: m[1], AimX: -0.6, AimY: 0.8}
		switch {
		case i == 3:
			f.Buttons, f.AimX, f.AimY = 1|1<<dashSlot, 0, -1
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
	var replays []int // the steps her client started over at: the first, and each after a surprise
	for i := range 2 * second {
		e, surprised := surprises[i]
		if surprised {
			fx.effect(a, e)
		}
		if i >= second-6 {
			bob.move(0, 1) // up across her second bolt's path
		}
		before, targets := bodiesFor(t, w, "alice"), sentTo[history.TargetSync](t, w, "alice")
		see(t, p, targets)
		if s := predictFrom(t, before); !p.Agrees(before.You.InputAck, s) {
			p.Reset(s)
			replays = append(replays, i)
		}
		f := frameAt(i)
		if i == second {
			// Aim where her screen will draw bob when the bolt gets there: at step k, his rows from k-3
			// and k-2, 0.7 of the way.
			m, b := moves[i%len(moves)], yourBody(t, bodiesFor(t, w, "bob"))
			launch := p.Peek(m[0], m[1])
			k := 0.0
			for range 20 {
				lead = box2d.Vec2{X: b.X - launch.X, Y: b.Y + float64(walk*(k+1-lag)) - launch.Y}
				k = math.Hypot(lead.X, lead.Y)/step - 0.5
			}
			f = frameAt(i)
		}
		f = alice.next(f)
		_, ended := p.Step(predictFrame(f))
		ends = append(ends, ended...)
		alice.send(f)
		w.Tick()
		after, afterRows, at := bodiesFor(t, w, "alice"), sentTo[history.TargetSync](t, w, "alice"), fmt.Sprintf("step %d", i)
		requirePredicted(t, at, guessOf(p), after, afterRows, !surprised)
		requireEnded(t, at, ended, w.Emitted[projectilecomp.ShotEnded](), a.EntityID)
		run = append(run, record(p, before, targets, []playercomp.InputFrame{f}, ended))
		p.Reset(predictFrom(t, before))
		_, replayed := p.Step(predictFrame(f))
		requirePredicted(t, at+" replayed", guessOf(p), after, afterRows, !surprised)
		requireEnded(t, at+" replayed", replayed, w.Emitted[projectilecomp.ShotEnded](), a.EntityID)
	}
	if want := []int{0, 3, 4, 7}; !slices.Equal(replays, want) {
		t.Errorf("her client started over at steps %v, want %v: the first, and the step after each surprise", replays,
			want)
	}
	if len(run[0].Shots) != 1 || len(run[second-1].Shots) != 0 {
		t.Fatalf("predicted %d shots after her first frame and %d before her second bolt, want one, then none",
			len(run[0].Shots), len(run[second-1].Shots))
	}
	if first, last := run[0].Echo, run[len(run)-1].Echo; first == nil || last == nil || *first == *last {
		t.Fatalf("predicted her Echo at %v after her first frame and %v after her last, want it moved", first, last)
	}
	// Her first bolt ends on wall 2 and her second on bob, each at its speed in its last tick.
	world := rules.Maps[bodiesFor(t, w, "alice").MapID]
	bobBody := yourBody(t, bodiesFor(t, w, "bob")).EntityID
	if len(ends) != 2 || len(touched(world, ends[0].End.At, ends[0].Radius+0.01, interact.Wall)) == 0 ||
		ends[1].End.Hit.Target != bobBody {
		t.Fatalf("ended %+v, want her first bolt on a wall, then her second on bob's Body %d", ends, bobBody)
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
			sentTo[history.TargetSync](t, w, "alice"), true)
		if shots := len(p.State().Shots); shots != 0 && i < 4 || shots != 1 && i >= 4 {
			t.Fatalf("replayed frame %d: %d shots predicted, want her bolt from frame 5 only", i+1, shots)
		}
	}

	run = append(run, badNetwork(t, w, p, predict.New(rules), alice, bolt)...)
	// Shots end on bob and elsewhere, after one frame and after several, so the wasm build meets each.
	pairs := map[[2]bool]bool{}
	for _, s := range run {
		for _, e := range s.Ended {
			pairs[[2]bool{len(s.Frames) > 1, e.OnTarget}] = true
		}
	}
	if len(pairs) != 4 {
		t.Fatalf("the run ends shots as (several frames, on target) %v, want all four", slices.Collect(maps.Keys(pairs)))
	}
	// Elsewhere is out of range, on a wall and on the edge. Her Body lands out of a wall, and slides on a wall
	// and along the edge from one segment to the next.
	var atRange, shotOnWall, shotOnEdge, landed, onWall, overJoint bool
	for _, s := range run {
		for _, e := range s.Ended {
			at, r := box2d.Vec2{X: e.X, Y: e.Y}, e.Radius+0.01
			wallHit, edgeHit := len(touched(world, at, r, interact.Wall)) > 0, len(touched(world, at, r, interact.Bounds)) > 0
			atRange = atRange || !e.OnTarget && !wallHit && !edgeHit
			shotOnWall, shotOnEdge = shotOnWall || !e.OnTarget && wallHit, shotOnEdge || !e.OnTarget && edgeHit
		}
		from, to, r := box2d.Vec2{X: s.Sync.You.X, Y: s.Sync.You.Y}, box2d.Vec2{X: s.X, Y: s.Y}, s.Sync.You.Radius+0.01
		inFrom, inTo := len(touched(world, from, 0, interact.Wall)) > 0, len(touched(world, to, 0, interact.Wall)) > 0
		landed = landed || inFrom && !inTo && box2d.Distance(from, to) > walk // faster than she walks out
		onWall = onWall || len(touched(world, to, r, interact.Wall)) > 0
		edgeFrom, edgeTo := touched(world, from, r, interact.Bounds), touched(world, to, r, interact.Bounds)
		overJoint = overJoint || len(edgeFrom) > 0 && len(edgeTo) > 0 && !maps.Equal(edgeFrom, edgeTo)
	}
	if !atRange || !shotOnWall || !shotOnEdge || !landed || !onWall || !overJoint {
		t.Fatalf("the run ends shots out of range %v, on a wall %v and on the edge %v; her Body lands %v, slides on "+
			"a wall %v and over an edge joint %v: want each", atRange, shotOnWall, shotOnEdge, landed, onWall,
			overJoint)
	}

	// The run's digest, a line a step, is the golden, so a change to the sim shows as a reviewed diff. Each
	// tick rate has its own. -record writes the run itself, for the wasm build to match (predictor.mjs).
	digestPath := filepath.Join("testdata", fmt.Sprintf("prediction-%dhz.txt", physics.ServerTickRateHz))
	var b strings.Builder
	for _, s := range run {
		b.WriteString(s.digest())
	}
	switch {
	case *recordTo != "":
		texts := map[string]string{}
		for name, raw := range files {
			texts[name] = string(raw)
		}
		raw, err := msgpack.Marshal(recording{Content: texts, Steps: run})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(*recordTo, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	case *update:
		if err := os.WriteFile(digestPath, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	default:
		golden, err := os.ReadFile(digestPath)
		if err != nil {
			t.Fatal(err)
		}
		want, got := strings.SplitAfter(string(golden), "\n"), strings.SplitAfter(b.String(), "\n")
		for i := range max(len(want), len(got)) {
			if i >= len(want) || i >= len(got) || want[i] != got[i] {
				t.Fatalf("%s line %d differs from this run. If the sim changed on purpose, run "+
					"`go test ./shards/game/test -run TestPredictionMatchesServer -update` (with -tags rate60 at 60 Hz) "+
					"and review the diff.\nwant: %.300s\ngot:  %.300s", digestPath, i+1, at(want, i), at(got, i))
			}
		}
	}
	// Each step, started over from its sync after the wire, lands where it did.
	recorded := overWire(t, run)
	p.Forget()
	for i, s := range recorded {
		see(t, p, s.Targets)
		p.Reset(predictFrom(t, s.Sync))
		var ended []predict.Ended
		for _, f := range s.Frames {
			_, e := p.Step(predictFrame(f))
			ended = append(ended, e...)
		}
		got := record(p, s.Sync, s.Targets, s.Frames, ended)
		if got.X != s.X || got.Y != s.Y || !slices.Equal(got.Shots, s.Shots) || got.Kit.Tick != s.Kit.Tick ||
			!slices.Equal(got.Kit.Slots, s.Kit.Slots) || !slices.Equal(got.Ended, s.Ended) ||
			(got.Echo == nil) != (s.Echo == nil) || got.Echo != nil && *got.Echo != *s.Echo {
			t.Fatalf("step %d: predicted (%v, %v), shots %+v, ended %+v, kit %+v, Echo %+v; first (%v, %v), %+v, "+
				"%+v, %+v, %+v", i, got.X, got.Y, got.Shots, got.Ended, got.Kit, got.Echo, s.X, s.Y, s.Shots, s.Ended,
				s.Kit, s.Echo)
		}
	}
}

// badNetwork plays alice's client through a scripted network, holding fire at bob as she walks. Each
// tick of the script: '.' her client makes a frame and sends it, '0' its clock runs slow and makes none,
// '2' it makes two after a hitch, 'l' the frame's Input is lost (the next one resends it), and 'h' the
// network holds it until the next '.' or '2'. Two more held in a row than the server's queue holds
// overflow it, and the slow ticks after drain it before her bolt is ready again. Each tick p starts over
// from the newest body_sync and replays her unacked frames. Whenever the server then applies those
// frames a tick each, it lands where the replay did, to the bit, its shots' ends included. A correction
// comes only from a surprise: at most a tick of walking for each tick the server coasted or frame it
// dropped. Her client's predictor q replays only when the server disagrees with it (Agrees), and lands
// where p does each tick: so only after a surprise. It returns each tick's replay, for the recording.
func badNetwork(t *testing.T, w *cardinal.TestWorld, p, q *predict.Predictor, alice *client, bolt flight.Shot,
) []predictionStep {
	t.Helper()
	before, targets := bodiesFor(t, w, "alice"), sentTo[history.TargetSync](t, w, "alice")
	queue, gap := int(before.You.InputCap), strings.Repeat(".", ticks(0.2))
	head := strings.Repeat(".", ticks(0.3)) + strings.Repeat("h", queue+2) + "." + strings.Repeat("0", queue-1) +
		strings.Repeat(".", ticks(0.3))
	// From the lost frame on, the server stays a frame behind her until her bolt is ready again and has
	// flown to bob, so a replay of several frames ends it on him at any rate.
	i := before.You.Cooldowns.IndexFunc(func(c cast.Cooldown) bool { return c.Ability == "bolt" })
	ready := int(before.You.Cooldowns.At(i).ReadyTick - before.Tick)
	a, b := yourBody(t, before), yourBody(t, bodiesFor(t, w, "bob"))
	flight := ticks(math.Hypot(b.X-a.X, b.Y-a.Y)/bolt.Speed + 0.25)
	script := head + "l" + strings.Repeat(".", max(len(gap), ready+flight-len(head))) + "2" + gap + "l" + gap + "0" +
		gap + "0" + gap + "h.h" + strings.Repeat(".", ticks(0.6))
	resend := ticks(0.4) // frames each Input repeats: ResendSeconds in client/src/game/player/input_stream.ts
	moves := [][2]float64{{1, 0}, {0.6, 0.8}, {0, 1}, {-1, 0}, {0, -1}, {1, 1}}
	walk, me := before.You.MoveSpeed*physics.SecondsPerTick, before.You.Body
	type replay struct { // a replay's guess, until the server applies its frame
		seq, ack uint32
		from     uint64 // the body_sync's tick it started from
		guess    guess
		ended    []predict.Ended
	}
	var (
		run                             []predictionStep
		open                            []replay
		unacked                         []playercomp.InputFrame
		held                            []player.Input
		drawn                           box2d.Vec2 // her Body after her newest frame, as last predicted
		surprises                       int        // since her client last predicted
		exact, deep, coasts, dropped    int
		deepHits, deepMisses, corrected int
		replays                         int // her client's
	)
	for i, c := range script {
		at := fmt.Sprintf("bad network tick %d", i)
		var ended []predict.Ended // every shot p's steps ended this tick
		// p starts over from the newest body_sync and replays what the server hasn't applied.
		see(t, p, targets)
		see(t, q, targets)
		p.Reset(predictFrom(t, before))
		ack := before.You.InputAck
		unacked = slices.DeleteFunc(unacked, func(f playercomp.InputFrame) bool { return f.Seq <= ack })
		for _, f := range unacked {
			_, e := p.Step(predictFrame(f))
			ended = append(ended, e...)
			open = append(open, replay{f.Seq, ack, before.Tick, guessOf(p), e})
		}
		if s := predictFrom(t, before); !q.Agrees(ack, s) {
			if i > 0 && surprises == 0 {
				t.Fatalf("%s: her client replayed with no surprise", at)
			}
			replays++
			q.Reset(s)
			for _, f := range unacked {
				q.Step(predictFrame(f))
			}
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
			_, e := p.Step(predictFrame(f))
			q.Step(predictFrame(f))
			ended = append(ended, e...)
			open = append(open, replay{f.Seq, ack, before.Tick, guessOf(p), e})
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
		if got, want := fmt.Sprintf("%+v", guessOf(q)), fmt.Sprintf("%+v", guessOf(p)); got != want { // floats exactly
			t.Fatalf("%s: her client has %s, a replay %s", at, got, want)
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
		rows, ends := sentTo[history.TargetSync](t, w, "alice"), w.Emitted[projectilecomp.ShotEnded]()
		waiting := open[:0]
		for _, r := range open {
			switch {
			case uint64(applied-r.ack) != after.Tick-r.from:
			case applied == r.seq:
				requirePredicted(t, at, r.guess, after, rows, true)
				requireEnded(t, at, r.ended, ends, me)
				exact++
				if r.seq-r.ack > 1 {
					deep++
					for _, e := range r.ended {
						if e.End.Hit.OnTarget() {
							deepHits++
						} else {
							deepMisses++
						}
					}
				}
			default:
				waiting = append(waiting, r)
			}
		}
		open = waiting
		before, targets = after, sentTo[history.TargetSync](t, w, "alice")
	}
	if exact < len(script) || deep == 0 || deepHits == 0 || deepMisses == 0 || coasts < 3 || dropped == 0 ||
		corrected == 0 || replays < 2 {
		t.Fatalf("bad network: %d exact replays, %d of several frames, ending %d shots on bob and %d elsewhere; "+
			"%d coasts, %d frames dropped, %d corrections, %d replays by her client: want each", exact, deep,
			deepHits, deepMisses, coasts, dropped, corrected, replays)
	}
	t.Logf("bad network: her client replayed %d of %d ticks", replays, len(script))
	return run
}

// The predictor sees a target only from the target syncs of every tick the views of a frame's tick are
// drawn from (past.Across), and only while the newest sync shows it alive. Otherwise her shot passes
// where her screen drew it. A hit ends her shot touching it where her screen drew it when they met, its
// last tick at its speed (test content: a bolt that flies 0.75 units a tick at any rate).
func TestPredictorPlacesOnlyWhatSyncsShow(t *testing.T) {
	_, bolt := primary(t)
	bolt.Speed = 0.75 * physics.ServerTickRateHz
	rules, err := predict.Load(patched(t, deepWindow, map[string]string{
		"projectile/tuning.json": fmt.Sprintf(`{"shots": {"bolt": {"speed": %v}}}`, bolt.Speed),
	}))
	if err != nil {
		t.Fatal(err)
	}
	step := bolt.Speed * physics.SecondsPerTick
	back := rules.History.Window.Ticks()
	now := back + 4 // her state's tick: for her first frame the server keeps rows from tick 5
	// The enemy stands on her shot's path, or farther on it, or 2 units up off it, or dies halfway
	// through a tick there. A newer one can take its ID, never its serial.
	enemy := predict.Sight{ID: 7, Serial: 70, Row: past.Row{Kind: interact.Body, Side: interact.Side{Owner: 7, Team: 1},
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
	// syncs is a sync of each tick from, showing each sight, or nothing for none.
	syncs := func(from uint64, sights ...*predict.Sight) map[uint64]*predict.Sight {
		m := map[uint64]*predict.Sight{}
		for i, s := range sights {
			m[from+uint64(i)] = s
		}
		return m
	}
	kept := map[uint64]*predict.Sight{}
	for tick := range back + 1 {
		kept[4+tick] = &enemy
	}
	keptUp := map[uint64]*predict.Sight{}
	for tick := range back + 1 {
		keptUp[4+tick] = map[bool]*predict.Sight{true: &farUp, false: &far}[tick < 2]
	}
	for _, c := range []struct {
		name  string
		syncs map[uint64]*predict.Sight
		view  float64
		hits  bool
	}{
		{"seen", syncs(10, &enemy, &enemy, &enemy), 10.5, true},
		{"seen a tick's flight away", syncs(10, &far, &far, &far, &far), 10.5, true},
		{"no sync of the oldest tick", syncs(11, &enemy, &enemy), 10.5, false},
		{"no sync of the newest tick", syncs(10, &enemy, &enemy), 10.5, false},
		{"gone from the newest", syncs(10, &enemy, &enemy, &enemy, nil), 10.5, false},
		{"drawn coming down onto the path", syncs(10, &up, &enemy, &enemy), 10.25, true},
		{"drawn leaving the path", syncs(10, &enemy, &up, &up, &up), 10.25, false},
		// The server removes it at the end of the tick it died in, before her frames.
		{"dead before her frames, though drawn alive", syncs(10, &enemy, &dying, nil), 10.25, false},
		{"died in the newest sync's tick", syncs(10, &enemy, &enemy, &dying), 10.25, false},
		// Judged at the oldest view the server keeps, 5, drawn from syncs 5 and 6: the predictor keeps both.
		{"drawn before the rows the server keeps", kept, 1.5, true},
		// Each step of a replay is held at its own tick: her second is judged at 6, where the enemy stood on
		// her path; at 5 it was 3 units up.
		{"drawn before the rows the server keeps, a step later", keptUp, 1.5, true},
		// Held at the newest view whose tick's views have their rows, now - 1.
		{"drawn after them", syncs(now-2, &far, &enemy, &enemy), float64(now) + 0.5, true},
		// As the server does, it draws the newer one where its first row is, not from the one it took the ID of.
		{"new with a dead one's ID", syncs(10, &diedUp, &newer, &newer), 10.25, true},
	} {
		p := predict.New(rules)
		for _, tick := range slices.Sorted(maps.Keys(c.syncs)) { // in tick order, as they arrive
			s := c.syncs[tick]
			var sights []predict.Sight
			if s != nil {
				sights = append(sights, *s)
			}
			p.See(tick, sights)
		}
		p.Reset(predict.State{Tick: now, Map: "open", Body: 1, Radius: 0.5, MoveSpeed: 5, Champion: "starter"})
		var ended []predict.Ended
		var view float64                         // judged at, in the step it ended
		for i, buttons := range []uint32{1, 0} { // she casts, then holds, her view a tick on
			_, stepEnded := p.Step(predict.Frame{Seq: uint32(i + 1), AimX: 1, Buttons: buttons, ViewTick: c.view + float64(i)})
			if len(stepEnded) > 0 {
				view = rules.History.Window.Clamp(c.view+float64(i), now+uint64(i)+1)
			}
			ended = append(ended, stepEnded...)
		}
		if hit := len(ended) == 1 && ended[0].End.Hit.Target == enemy.ID; hit != c.hits || len(ended) > 1 {
			t.Errorf("%s: ended %+v, want a hit on %d: %v", c.name, ended, enemy.ID, c.hits)
			continue
		}
		if !c.hits {
			continue
		}
		// Where her screen drew it when they met: at its tick's view, Until of the way on. It is the one the
		// newest sync shows, by its serial.
		e := ended[0]
		if newest := c.syncs[slices.Max(slices.Collect(maps.Keys(c.syncs)))]; e.End.Hit.Serial != newest.Serial {
			t.Errorf("%s: hit serial %d, want the newest sync's, %d", c.name, e.End.Hit.Serial, newest.Serial)
		}
		met := view + e.End.Until
		older, newer := past.RowTicks(met)
		var o, n past.Row
		if s := c.syncs[older]; s != nil && s.Serial == e.End.Hit.Serial {
			o = s.Row
		}
		if s := c.syncs[newer]; s != nil {
			n = s.Row
		}
		drawn, _ := past.AtView(o, o != past.Row{}, n, met)
		at, center := e.End.At, box2d.Sub(e.End.At, e.End.Hit.Offset)
		// Box2D stops a cast a slop short of touching.
		if box2d.Distance(center, drawn.Center) > 1e-9 ||
			math.Abs(box2d.Length(e.End.Hit.Offset)-(bolt.Radius+enemy.Radius)) > 2*box2d.LinearSlop ||
			math.Abs(box2d.Distance(at, e.Pos)-e.End.Until*step) > 1e-9 {
			t.Errorf("%s: ended %+v, want her shot touching the enemy where she drew it then, %v, at its speed", c.name,
				e, drawn.Center)
		}
	}
	// A sync older than the rows kept changes nothing, though it comes last and shares the newest's slot.
	p := predict.New(rules)
	for _, tick := range slices.Sorted(maps.Keys(kept)) {
		p.See(tick, []predict.Sight{*kept[tick]})
	}
	p.See(4, []predict.Sight{farUp})
	p.Reset(predict.State{Tick: now, Map: "open", Body: 1, Radius: 0.5, MoveSpeed: 5, Champion: "starter"})
	if _, ended := p.Step(predict.Frame{Seq: 1, AimX: 1, Buttons: 1, ViewTick: 1.5}); len(ended) != 1 ||
		ended[0].End.Hit.Target != enemy.ID {
		t.Errorf("a sync older than the rows kept came last: ended %+v, want a hit on %d", ended, enemy.ID)
	}
	// A frame stepped before its views' syncs came, or after her client forgot them, flew her shot past
	// every target, so her client replays it, though the server agrees with the frame before.
	for _, c := range []struct {
		view           float64
		forgot, agrees bool
	}{{10.5, false, true}, {9.5, false, false}, {10.5, true, false}} {
		p := predict.New(rules)
		for tick := range uint64(3) {
			p.See(10+tick, []predict.Sight{far})
		}
		p.Reset(predict.State{Tick: now, Map: "open", Body: 1, Radius: 0.5, MoveSpeed: 5, Champion: "starter"})
		p.Step(predict.Frame{Seq: 1, AimX: 1, Buttons: 1, ViewTick: 10.5})
		first := p.State()
		if c.forgot {
			p.Forget()
		}
		p.Step(predict.Frame{Seq: 2, AimX: 1, ViewTick: c.view})
		if got := p.Agrees(1, first); got != c.agrees {
			t.Errorf("her second frame drawn at %v, after forgetting %v: agrees %v, want %v", c.view, c.forgot, got,
				c.agrees)
		}
	}
}

// Past the path her last state sent, her Echo is a guess: where that path ends. A state that has it
// elsewhere at a tick her frames stepped disagrees, though all else agrees. One that differs only after
// them agrees, and her Echo takes its path.
func TestPredictorAgreesOnlyWithTheEchoItStepped(t *testing.T) {
	rules, err := predict.Load(content.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		path   []box2d.Vec2 // the server's, from tick 11
		agrees bool
	}{
		{"as guessed", []box2d.Vec2{{X: 1}, {X: 1}, {X: 1}}, true},
		{"elsewhere after her frames", []box2d.Vec2{{X: 1}, {X: 1}, {X: 2}}, true},
		{"elsewhere at her second frame", []box2d.Vec2{{X: 1}, {X: 1.5}, {X: 2}}, false},
	} {
		p := predict.New(rules)
		p.Reset(predict.State{Tick: 10, Map: "open", Body: 1, Radius: 0.5, MoveSpeed: 5, Champion: "starter",
			EchoDelay: 5, EchoPath: []box2d.Vec2{{}, {X: 1}}})
		p.Step(predict.Frame{Seq: 1})
		first := p.State()
		p.Step(predict.Frame{Seq: 2})
		first.EchoPath = c.path
		if got := p.Agrees(1, first); got != c.agrees {
			t.Errorf("%s: agrees %v, want %v", c.name, got, c.agrees)
		} else if at, _, _ := p.Echo(); got && at != c.path[1] {
			t.Errorf("%s: her Echo at %v, want the server's %v", c.name, at, c.path[1])
		}
	}
}

// Your predictor walks the map its state names, as body_sync names the live one: the same frames stop
// her at a wall on one map, and walk her on through the other, which has none there.
func TestPredictorWalksTheMapItsStateNames(t *testing.T) {
	const r, gap = 0.5, 0.25 // her radius, and her edge to the wall's
	files := onMap(t, content.Overlay{}, level.Map{Solids: []level.Solid{
		wall(box2d.Vec2{X: r + gap + 0.25, Y: 0}, box2d.Vec2{X: 0.25, Y: 2}),
	}})
	files["arena/tuning.json"] = []byte(`{"maps": ["open", "test"], "default_map": "test"}`)
	rules, err := predict.Load(files)
	if err != nil {
		t.Fatal(err)
	}
	p := predict.New(rules)
	walked := map[string]float64{}
	for _, id := range []string{"test", "open"} {
		p.Reset(predict.State{Tick: 10, Map: id, Body: 1, Radius: r, MoveSpeed: 5, Champion: "starter"})
		for seq := range uint32(ticks(0.5)) {
			p.Step(predict.Frame{Seq: seq + 1, MoveX: 1})
		}
		walked[id] = p.State().Center.X
	}
	if math.Abs(walked["test"]-gap) > 0.01 || walked["open"] <= 1 {
		t.Fatalf("walked to x = %v on the walled map and %v on the open one, want %v at the wall, then on past it",
			walked["test"], walked["open"], gap)
	}
}

// sharedCore is the code the client predictor links from this module and world-engine (box2d), all but
// its js-only glue: the code that must give every target the same bits. Taken from the build, so a new
// import is scanned the day it lands.
func sharedCore(t *testing.T) []string {
	list := exec.Command("go", "list", "-deps", "../predictor")
	list.Env = append(list.Environ(), "GOOS=js", "GOARCH=wasm")
	deps, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, deps)
	}
	var core []string
	for dep := range strings.Lines(string(deps)) {
		dep = strings.TrimSpace(dep)
		if strings.HasPrefix(dep, "github.com/argus-labs/world-engine/pkg/cardinal") {
			t.Errorf("the predictor imports %s", dep) // a component leaf pulls in about 737 packages
		}
		if (strings.HasPrefix(dep, "echo_image/") || strings.HasPrefix(dep, "github.com/argus-labs/world-engine/")) &&
			dep != "echo_image/shards/game/predictor" {
			core = append(core, dep)
		}
	}
	return core
}

// exactMath is the math functions the shared core may call: their results are exact, so no target
// rounds them differently. The rest of math isn't scanned, and some of it fuses (math.Hypot on arm64).
var exactMath = map[string]bool{"remainder": true, "mod": true, "modf": true, "floor": true, "ceil": true,
	"trunc": true, "round": true, "roundtoeven": true, "frexp": true, "ldexp": true}

// The shared core imports no Cardinal (a component leaf pulls in about 737 packages), and builds with
// no fused multiply-add on targets that have one. An FMA rounds a*b+c once, so it gives other bits
// than a target that can't fuse, like wasm. Fix a hit by rounding the product: float64(a*b). It calls
// only exactMath of the math package: use box2d's FMA-free helpers (box2d.Length) for the rest.
func TestSharedCoreIsDeterministic(t *testing.T) {
	core := sharedCore(t)
	fma := regexp.MustCompile(`\bV?FN?M(ADD|SUB)[0-9A-Z]*\b`) // FMADDD on arm64, VFMADD231SD on amd64/v3
	mathCall := regexp.MustCompile(`\bCALL\s+math\.(\w+)`)
	for _, target := range [][]string{{"GOOS=linux", "GOARCH=arm64"}, {"GOOS=linux", "GOARCH=amd64", "GOAMD64=v3"},
		{"GOOS=js", "GOARCH=wasm"}} {
		build := exec.Command("go", append([]string{"build", "-gcflags=-S"}, core...)...) // -S prints assembly
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
