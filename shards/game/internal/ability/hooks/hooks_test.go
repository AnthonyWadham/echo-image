package hooks

import (
	"encoding/json"
	"slices"
	"testing"

	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/physics/timestep"
)

// A bad hook name, an unknown param or a bad value fails the bind, so bad content fails the boot.
func TestBindRejectsBadContent(t *testing.T) {
	for _, c := range []struct{ name, params string }{
		{"lunch", `{"shot": "bolt"}`},
		{"launch", `{"shoot": "bolt"}`},
		{"launch", `{}`},
		{"launch", ``},
		{"dash", `{"distance": 6, "seconds": 0.005, "effect": "dash"}`}, // under one tick at 20 and 60 Hz
	} {
		if _, err := Bind(c.name, json.RawMessage(c.params)); err == nil {
			t.Errorf("Bind(%q, %s) worked; want an error", c.name, c.params)
		}
	}
}

// launch fires its shot along the aim, whoever casts it: here an Echo, for its Body.
func TestLaunchFiresAlongAim(t *testing.T) {
	h, err := Bind("launch", json.RawMessage(`{"shot": "bolt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !h.Aimed || !slices.Equal(h.Refs.Shots, []string{"bolt"}) {
		t.Fatalf("hook %+v, want an aimed one that names shot bolt", h)
	}
	var out Out
	h.OnCast(Cast{Caster: 9, Owner: 2, Team: 1, Pos: Vec2{3, 4}, Aim: Vec2{0, 1}}, &out)
	if want := []Launch{{Shot: "bolt", Dir: Vec2{0, 1}}}; !slices.Equal(out.Launches, want) {
		t.Fatalf("launches %+v, want %+v", out.Launches, want)
	}
}

// dash moves its caster its distance in whole ticks: 6 m in 0.3 s is 20 m/s at any rate.
func TestDashCoversItsDistance(t *testing.T) {
	h, err := Bind("dash", json.RawMessage(`{"distance": 6, "seconds": 0.3, "effect": "dash"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out Out
	h.OnCast(Cast{Caster: 9, Owner: 2, Aim: Vec2{-0.6, 0.8}}, &out)
	ticks := uint64(timestep.Ticks(0.3))
	want := []status.Intent{{Target: 9, Kind: "dash", Ticks: ticks, VelX: -0.6 * 20, VelY: 0.8 * 20}}
	if !slices.Equal(out.Effects, want) || !slices.Equal(h.Refs.Effects, []string{"dash"}) {
		t.Fatalf("effects %+v and refs %+v, want %+v naming dash", out.Effects, h.Refs, want)
	}
}
