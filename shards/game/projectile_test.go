package main

import (
	abilitycomp "echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/ability/hooks"
	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/projectile"
	projectilecomp "echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
	"fmt"
	"maps"
	"math"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// firstTouch is the step, counted from the one c's shot left on, whose path first touches a circle of
// radius r that lies dist from where it left, along its path. Its speed and size come from body_sync.
func firstTouch(t *testing.T, w *cardinal.TestWorld, c *client, dist, r float64) int {
	t.Helper()
	s := bodiesFor(t, w, c.id).You.Shots.At(0)
	return int((dist - r - s.Radius) / (math.Hypot(s.VelX, s.VelY) * physics.SecondsPerTick))
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

// Shots pass allies: alice's shot flies through carol, her teammate 2 m in front of her, and carol's
// Echo, where she drew them.
func TestShotPassesThroughAllies(t *testing.T) {
	w := cardinal.NewTestWorld(t, withContent(t, twoVsTwo))
	alice, _ := claim(t, w)
	join(t, w, "carol")
	from := yourBody(t, bodiesFor(t, w, "alice"))
	flyFor(w, int(math.Ceil(drawLag))+1) // carol's rows reach back to alice's view

	// The shot starts inside alice's own Body and Echo, then crosses carol and her Echo.
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
	w := cardinal.NewTestWorld(t, withEmitter(withContent(t, effectKinds, map[string]string{
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
