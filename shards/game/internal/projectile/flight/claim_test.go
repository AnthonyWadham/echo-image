package flight

import (
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
)

// A target at a view is drawn as the client draws it: its newer row, partway from its older one. One
// that died in the newer tick moves at its own pace to where it died, and is gone after.
func TestAtViewDrawsAsTheClientDoes(t *testing.T) {
	older := Row{Tick: 10, Center: box2d.Vec2{X: 1, Y: 2}, Radius: 0.5, Tags: interact.Stunned, Until: 1}
	at := func(x, y float64) *box2d.Vec2 { return &box2d.Vec2{X: x, Y: y} }
	for _, c := range []struct {
		name     string
		hasOlder bool
		until    float64 // the newer row's
		view     float64
		want     *box2d.Vec2 // nil if gone
	}{
		{"a quarter of the way", true, 1, 10.25, at(1.5, 1)},
		{"on the older tick", true, 1, 10, at(1, 2)},
		{"new at the newer tick", false, 1, 10.25, at(3, -2)},
		{"a quarter of the way, dying halfway", true, 0.5, 10.25, at(2, 0)},
		{"where it died", true, 0.5, 10.5, at(3, -2)},
		{"after it died", true, 0.5, 10.75, nil},
		{"dead at the tick's start", true, 0, 10, at(3, -2)},
		{"new, after it died", false, 0.5, 10.75, nil},
	} {
		newer := Row{Tick: 11, Center: box2d.Vec2{X: 3, Y: -2}, Radius: 0.5, Tags: interact.CC, Until: c.until}
		got, ok := AtView(older, c.hasOlder, newer, c.view)
		want := newer
		if c.want != nil {
			want.Center = *c.want
		}
		if ok != (c.want != nil) || ok && got != want {
			t.Errorf("%s: %+v %v, want %+v %v", c.name, got, ok, want, c.want != nil)
		}
	}
}

// A claim's view is at most RewindTicks old, and its rows must be saved before the claim's tick.
func TestValidView(t *testing.T) {
	tuning := Tuning{RewindSeconds: 0.4}
	tick := uint64(100)
	cap := float64(tick - tuning.RewindTicks())
	for _, c := range []struct {
		view float64
		want bool
	}{
		{cap, true},
		{cap - 0.01, false},
		{98.5, true}, // rows 98 and 99 are saved
		{99, false},  // needs row 100
		{math.NaN(), false},
		{math.Inf(1), false},
		{math.Inf(-1), false},
	} {
		if got := tuning.ValidView(c.view, tick); got != c.want {
			t.Errorf("ValidView(%v, %d) = %v, want %v", c.view, tick, got, c.want)
		}
	}
	if tuning.ValidView(-0.5, 3) {
		t.Error("a view before tick 0 is valid")
	}
}
