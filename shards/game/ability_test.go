package main

import (
	"echo_image/shards/game/internal/ability"
	abilitycomp "echo_image/shards/game/internal/ability/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	playercomp "echo_image/shards/game/internal/player/component"
	projectilecomp "echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/team"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

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
	w := cardinal.NewTestWorld(t, register)
	def, _ := primary(t)
	cooldown := def.CooldownTicks()

	// Her first frame claims her Body and fires: Equip runs before Cast, so a Body casts the tick it's
	// claimed.
	alice := &client{w: w, id: "alice", session: 1, back: drawLag}
	(&client{w: w, id: "bob", session: 1, back: drawLag}).move(0, 0)
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
