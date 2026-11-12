package test

import (
	"echo_image/shards/game/internal/area"
	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/player"
	"echo_image/shards/game/shard"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

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
		{"shipped", shard.Register, echoDelay(t)},
		{"quick", quick, 2 * physics.ServerTickRateHz},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := cardinal.NewTestWorld(t, c.setup)
			alice, _ := claim(t, w)
			spawn := yourBody(t, bodiesFor(t, w, "alice"))
			var path []body
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

// A player whose Echo died is out for the round (GDD §8): they can't cast, and shots pass their Body, so
// it shields no one.
func TestEchoDeathPutsItsPlayerOut(t *testing.T) {
	w, alice, bob, fx := fxDuel(t)
	a := yourBody(t, bodiesFor(t, w, "alice"))
	mine := echoOf(t, w, "alice", a)
	emit(fx, combatcomp.DamageIntent{Target: cardinal.EntityID(mine.EntityID), Amount: mine.MaxHP})
	w.Tick() // her Echo dies; out applies from the next tick

	b := yourBody(t, bodiesFor(t, w, "bob"))
	alice.fire(1, 0)
	bob.fire(a.X-b.X, a.Y-b.Y) // at her
	w.Tick()
	if hers, his := shots(t, w, "bob"), shots(t, w, "alice"); len(hers) != 0 || len(his) != 1 || his[0].Caster != b.EntityID {
		t.Fatalf("bob sees shots %+v and alice %+v, want none and bob's: she is out", hers, his)
	}
	ended := flyUntilEnded(t, w, "alice", ticks(5))
	if len(ended) != 1 || ended[0].OnTarget {
		t.Fatalf("bob's shot ended %+v, want it past her Body", ended)
	}
}
