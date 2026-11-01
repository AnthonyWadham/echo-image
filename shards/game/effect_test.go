package main

import (
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

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
