package main

import (
	"cmp"
	echoimage "echo_image"
	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/echo"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/predict"
	"echo_image/shards/game/internal/projectile"
	projectilecomp "echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
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
// testdata/prediction-20hz.txt (one per tick rate) is the digest of the run and the bad network's replays.
// -record writes the run, so the client checks its wasm build lands there too (predictor.test.ts).
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
	// Shots end out of range and on bob, after one frame and after several, so the wasm build meets each.
	pairs := map[[2]bool]bool{}
	for _, s := range run {
		for _, e := range s.Ended {
			pairs[[2]bool{len(s.Frames) > 1, e.OnTarget}] = true
		}
	}
	if len(pairs) != 4 {
		t.Fatalf("the run ends shots as (several frames, on target) %v, want all four", slices.Collect(maps.Keys(pairs)))
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
		raw, err := msgpack.Marshal(run)
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
					"`go test ./shards/game -run TestPredictionMatchesServer -update` (with -tags rate60 at 60 Hz) "+
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
// overflow it, and the slow ticks after drain it before her bolt is ready again. Each tick her client
// starts over from the newest body_sync and replays her unacked frames, as it does. Whenever the server
// then applies those frames a tick each, it lands where the replay did, to the bit, its shots' ends
// included. A correction comes only from a surprise: at most a tick of walking for each tick the server
// coasted or frame it dropped. It returns each tick's replay, for the recording.
func badNetwork(t *testing.T, w *cardinal.TestWorld, p *predict.Predictor, alice *client) []predictionStep {
	t.Helper()
	before, targets := bodiesFor(t, w, "alice"), sentTo[projectile.TargetSync](t, w, "alice")
	queue, gap := tuningOf[player.MovementTuning](t).BufferFrames(), strings.Repeat(".", ticks(0.2))
	head := strings.Repeat(".", ticks(0.3)) + strings.Repeat("h", queue+2) + "." + strings.Repeat("0", queue-1) +
		strings.Repeat(".", ticks(0.3))
	// From the lost frame on, the server stays a frame behind her until her bolt is ready again and has
	// flown to bob, so a replay of several frames ends it on him at any rate.
	_, bolt := primary(t)
	i := before.You.Cooldowns.IndexFunc(func(c cast.Cooldown) bool { return c.Ability == kit(t)[0] })
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
	)
	for i, c := range script {
		at := fmt.Sprintf("bad network tick %d", i)
		var ended []predict.Ended // every shot her client's steps ended this tick
		// Her client starts over from the newest body_sync and replays what the server hasn't applied.
		see(t, p, targets)
		p.Reset(predictFrom(t, before))
		ack := before.You.InputAck
		unacked = slices.DeleteFunc(unacked, func(f playercomp.InputFrame) bool { return f.Seq <= ack })
		for _, f := range unacked {
			_, e := p.Step(predictFrame(f))
			ended = append(ended, e...)
			open = append(open, replay{f.Seq, ack, before.Tick, guessOf(p), e})
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
		before, targets = after, sentTo[projectile.TargetSync](t, w, "alice")
	}
	if exact < len(script) || deep == 0 || deepHits == 0 || deepMisses == 0 || coasts < 3 || dropped == 0 ||
		corrected == 0 {
		t.Fatalf("bad network: %d exact replays, %d of several frames, ending %d shots on bob and %d elsewhere; "+
			"%d coasts, %d frames dropped, %d corrections: want each", exact, deep, deepHits, deepMisses, coasts,
			dropped, corrected)
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

// sharedCore is the code the client predictor links from this module and world-engine (box2d), all but
// its js-only glue: the code that must give every target the same bits. Taken from the build, so a new
// import is scanned the day it lands.
func sharedCore(t *testing.T) []string {
	list := exec.Command("go", "list", "-deps", "./predictor")
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
