package test

import (
	combatcomp "echo_image/shards/game/internal/combat/component"
	"echo_image/shards/game/internal/echo"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// HP stays between 0 and MaxHP. Damage lands first, then heals, and deaths only after both, so a
// same-tick heal saves a target if it covers the overkill.
func TestHealthStaysBetweenZeroAndMaxHP(t *testing.T) {
	w, _, _, fx := fxDuel(t)
	bobs := func() echoSeen { return echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "bob"))) }
	e := bobs()
	target := cardinal.EntityID(e.EntityID)

	send(fx, combatcomp.DamageIntent{Target: target, Amount: 30})
	send(fx, combatcomp.HealIntent{Target: target, Amount: 100})
	w.Tick()
	if got := bobs().HP; got != e.MaxHP {
		t.Fatalf("HP = %v after 30 damage and a 100 heal, want MaxHP %v", got, e.MaxHP)
	}

	send(fx, combatcomp.HealIntent{Target: target, Amount: e.MaxHP}) // sent first, lands second
	send(fx, combatcomp.DamageIntent{Target: target, Amount: e.MaxHP + 10})
	w.Tick()
	if got := bobs().HP; got != e.MaxHP-10 {
		t.Fatalf("HP = %v after a lethal hit and a heal in one tick, want %v", got, e.MaxHP-10)
	}

	// Emitted out of time order: the second hit to land takes it to 0, though by size it'd be the third.
	send(fx, combatcomp.DamageIntent{Target: target, Amount: 30, Until: 0.75})
	send(fx, combatcomp.DamageIntent{Target: target, Amount: e.MaxHP - 20, Until: 0.25})
	send(fx, combatcomp.DamageIntent{Target: target, Amount: 20, Until: 0.5})
	mine := echoOf(t, w, "alice", yourBody(t, bodiesFor(t, w, "alice")))
	send(fx, combatcomp.HealIntent{Target: cardinal.EntityID(mine.EntityID), Amount: -mine.MaxHP}) // a bad heal
	w.Tick()
	// Sent once more, at HP 0, alive until the hit that took it there landed: no hit took hers.
	want := map[uint32]float64{e.EntityID: 0.5, mine.EntityID: 1}
	for other := range sentTo[echo.EchoSync](t, w, "alice").Echoes.Values() {
		if until, ok := want[other.EntityID]; ok {
			if row := rowOf(t, w, "alice", other.Serial); other.HP != 0 || row.Until != until {
				t.Fatalf("dead Echo %d sent at %v HP until %v, want 0 HP until %v", other.EntityID, other.HP, row.Until, until)
			}
			delete(want, other.EntityID)
		}
	}
	if len(want) != 0 {
		t.Fatalf("dead Echoes %v weren't sent once more", want)
	}
	w.Tick()
	for other := range sentTo[echo.EchoSync](t, w, "alice").Echoes.Values() {
		if other.EntityID == e.EntityID || other.EntityID == mine.EntityID {
			t.Fatalf("Echo %d is still here a tick after it died", other.EntityID)
		}
	}
}
