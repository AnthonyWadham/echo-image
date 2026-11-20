package test

import (
	"echo_image/shards/game/internal/ability"
	abilitycomp "echo_image/shards/game/internal/ability/component"
	effectcomp "echo_image/shards/game/internal/effect/component"
	playercomp "echo_image/shards/game/internal/player/component"
	projectilecomp "echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/team"
	"fmt"
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
func TestFireStartsCooldownAndOthersSeeTheShot(t *testing.T) {
	w := cardinal.NewTestWorld(t, withContent(t))
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
	// Bob sees it. Her client draws it from her predictor, so hers leaves it out.
	if len(shots(t, w, "alice")) != 0 || len(shots(t, w, "bob")) != 1 {
		t.Fatalf("alice sees %d shots and bob %d, want none and 1", len(shots(t, w, "alice")), len(shots(t, w, "bob")))
	}
	// It flies the whole tick.
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
	// Bob sees it end; hers leaves that out too.
	_, bolt := primary(t)
	if ended := flyUntilEnded(t, w, "bob", ticks(bolt.Range/bolt.Speed)+1); len(ended) != 1 ||
		len(endedShots(t, w, "alice")) != 0 {
		t.Fatalf("bob sees %d shots end and alice %d, want 1 and none", len(ended), len(endedShots(t, w, "alice")))
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
	w := cardinal.NewTestWorld(t, withFiles(twoVsTwo(t)))
	alice, _ := claim(t, w)
	join(t, w, "carol") // alice's team
	aliceBody := yourBody(t, bodiesFor(t, w, "alice"))

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
	if n := len(shots(t, w, "bob")); n != 0 {
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
	if n := len(shots(t, w, "bob")); n != 0 {
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
	if n := len(shots(t, w, "bob")); n != 0 {
		t.Fatalf("a stunned Body fired %d shots", n)
	}
	if ready, _ := readyTick(t, w, "alice", body.EntityID); ready != 0 {
		t.Fatalf("ReadyTick = %d, want 0: no cooldown spent", ready)
	}
}

// An ability's own blocked_by replaces the file's: one with none casts while stunned.
func TestOwnBlockedByLetsAStunnedBodyCast(t *testing.T) {
	fx := &emitter{}
	own := map[string]string{"ability/tuning.json": fmt.Sprintf(`{"abilities": {%q: {"blocked_by": []}}}`, kit(t)[0])}
	w := cardinal.NewTestWorld(t, withEmitter(withContent(t, effectKinds, own), fx))
	alice, _ := claim(t, w)
	body := yourBody(t, bodiesFor(t, w, "alice"))
	fx.effect(body, effectcomp.EffectIntent{Kind: "stun", Ticks: 5})
	w.Tick() // the stun applies from the next tick
	alice.fire(0, 1)
	w.Tick()
	if ready, _ := readyTick(t, w, "alice", body.EntityID); ready == 0 {
		t.Fatal("a stunned Body didn't cast an ability whose own blocked_by is empty")
	}
}

// Never assume the caster: every ability, cast by a Body or by an Echo, sends intents that carry the
// caster as the cause and start from it, for the caster's owner. Their owners drain them that tick:
// shots leave from where the caster was, and effects come from it.
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
	type shotRow struct {
		Projectile projectilecomp.Projectile
		Owner      team.Owner
		Team       team.Team
	}
	type holderRow struct {
		Effects effectcomp.Effects
	}
	// sources is the source of each effect entry in the world, by holder and serial.
	sources := func() map[[2]uint64]uint32 {
		all := map[[2]uint64]uint32{}
		for e := range w.Contains[holderRow]().Iter() {
			for x := range e.Get[effectcomp.Effects]().List.Values() {
				all[[2]uint64{uint64(e.ID()), uint64(x.Serial)}] = x.Source
			}
		}
		return all
	}
	for id := range tuningOf[ability.AbilityTuning](t).Abilities {
		for caster, cid := range casters {
			cause, before := abilitycomp.Cause{Caster: cid, Ability: id}, sources()
			emit(fx, abilitycomp.AbilityCast{Cause: cause, AimX: 0.6, AimY: 0.8})
			w.Tick()
			from, sent := where(caster), 0
			for e := range w.Contains[shotRow]().Iter() {
				if p := e.Get[projectilecomp.Projectile](); p.Cause == cause {
					sent++
					owner, side := e.Get[team.Owner]().Body, e.Get[team.Team]().ID
					if owner != cardinal.EntityID(body.EntityID) || side != body.Team || p.Prev != from {
						t.Errorf("%s cast by her %s: a shot from %v for %d on team %d, want it from %v for her Body %d",
							id, caster, p.Prev, owner, side, from, body.EntityID)
					}
				}
			}
			for key, source := range sources() {
				if _, ok := before[key]; ok {
					continue
				}
				sent++
				if source != uint32(cid) {
					t.Errorf("%s cast by her %s: an effect from %d, want from %d", id, caster, source, cid)
				}
			}
			if sent == 0 {
				t.Errorf("%s cast by her %s sent no intent", id, caster)
			}
		}
	}
}
