package levelstate_test

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"testing"

	"echo_image/pkg/levelstate"
)

// Init makes the default map live at epoch 1, and emits nothing: its events would reach the first tick
// even after a restore.
func TestInitMakesTheDefaultLive(t *testing.T) {
	cfg := config(t)
	w, p, s := newWorld(t, cfg)
	want := levelstate.Current{ID: "open", Hash: sha256.Sum256(cfg.Maps["open"]), Epoch: 1}
	if got := state(t, w); got != want || p.Engine().Counters().ShapeCount != 4 {
		t.Errorf("state %+v, want %+v and open's 4 shapes", got, want)
	}
	w.Tick()
	if len(s.changed) != 0 {
		t.Errorf("the first tick saw %+v, want nothing", s.changed)
	}
}

// A restore replaces state after Init. The first system of the next tick swaps memory to the map state
// names. It emits Changed only if the live map changed: a file that differs in this build, a change
// requested before the snapshot, or a snapshot from before the plugin.
func TestRestore(t *testing.T) {
	cfg := config(t)
	open, pillar := sha256.Sum256(cfg.Maps["open"]), sha256.Sum256(cfg.Maps["pillar"])
	for _, c := range []struct {
		name     string
		restored *levelstate.Current
		want     levelstate.Current
		shapes   int
		changed  []levelstate.Changed
	}{
		{
			"another map", &levelstate.Current{ID: "pillar", Hash: pillar, Epoch: 5},
			levelstate.Current{ID: "pillar", Hash: pillar, Epoch: 5}, 5, nil,
		},
		{
			"a map whose file changed", &levelstate.Current{ID: "pillar", Hash: sha256.Sum256(nil), Epoch: 5},
			levelstate.Current{ID: "pillar", Hash: pillar, Epoch: 6}, 5,
			[]levelstate.Changed{{Previous: "pillar", ID: "pillar", Epoch: 6}},
		},
		{
			"a change requested", &levelstate.Current{ID: "open", Hash: open, Epoch: 5, Next: "pillar"},
			levelstate.Current{ID: "pillar", Hash: pillar, Epoch: 6}, 5,
			[]levelstate.Changed{{Previous: "open", ID: "pillar", Epoch: 6}},
		},
		{
			"no map", nil,
			levelstate.Current{ID: "open", Hash: open, Epoch: 1}, 4,
			[]levelstate.Changed{{ID: "open", Epoch: 1}},
		},
	} {
		w, _, s := newWorld(t, cfg)
		if c.restored != nil {
			holder(t, w).Set(*c.restored)
		} else {
			holder(t, w).Destroy()
		}
		w.Tick()
		if got := state(t, w); got != c.want || s.shapes != c.shapes || !slices.Equal(s.changed, c.changed) {
			t.Errorf("%s: state %+v, %d shapes and %+v, want %+v, %d and %+v",
				c.name, got, s.shapes, s.changed, c.want, c.shapes, c.changed)
		}
	}
}

// A restored map the build lacks stops the tick: only a world purge clears it.
func TestRestoreOfAMapTheBuildLacksStops(t *testing.T) {
	w, _, _ := newWorld(t, config(t))
	holder(t, w).Set(levelstate.Current{ID: "cave", Epoch: 3})
	var got any
	func() {
		defer func() { got = recover() }()
		w.Tick()
	}()
	if want := `map "cave" isn't in this build; run world purge`; !strings.Contains(fmt.Sprint(got), want) {
		t.Errorf("got %v, want a panic with %q", got, want)
	}
}
