package main

import (
	areacomp "echo_image/shards/game/internal/area/component"
	combatcomp "echo_image/shards/game/internal/combat/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/team"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// addArea builds an entity of side s at pos holding an area of kind kind, with a 0.5 m circle, acting at
// once. It's like an Echo.
func addArea(w *cardinal.TestWorld, kind string, s body, pos physics2d.Vec2) cardinal.EntityID {
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
	w := cardinal.NewTestWorld(t, withEmitter(withContent(t, effectKinds, map[string]string{
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
