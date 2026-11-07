package flight

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/track/past"
)

// Fly's End is where the shot's center was and when in the tick, from the sweep: on a hit, on a hit in
// a last tick its range cut short, and at its range.
func TestFlyEndsWhereAndWhenTheSweepSays(t *testing.T) {
	var tuning Tuning
	if err := json.Unmarshal([]byte(`{"defaults": [{"kinds": ["echo"], "do": "stop"}]}`), &tuning); err != nil {
		t.Fatal(err)
	}
	def := Shot{Speed: 15, Range: 20, Radius: 0.15}
	const dt = 0.05 // a tick flies 0.75
	for _, c := range []struct {
		name      string
		echo      float64 // the Echo's center on the path, 0 for none
		remaining float64
		at, until float64 // where its center ends, and when
		hit       uint32
	}{
		{"a hit", 0.85, 20, 0.2, 0.2 / 0.75, 10},
		{"a hit in a last tick cut short", 0.85, 0.3, 0.2, 0.2 / 0.75, 10},
		{"its range", 0, 0.3, 0.3, 0.3 / 0.75, 0},
	} {
		worldDef := box2d.DefaultWorldDef()
		var things []thing
		if c.echo != 0 {
			at := box2d.Vec2{X: c.echo}
			things = append(things, thing{interact.Echo, interact.Side{Owner: 7, Team: 1}, at, at, at})
		}
		f := Flight{Velocity: box2d.Vec2{X: def.Speed}, Radius: def.Radius, Remaining: c.remaining}
		a := past.Actor{Side: interact.Side{Owner: 1}, View: judged}
		_, end, over := tuning.Fly(box2d.NewWorld(&worldDef), def, f, a, sceneOf(false, things...), dt)
		// Box2D stops a cast a slop short of touching.
		if !over || end.Hit.Target != c.hit || math.Abs(end.At.X-c.at) > 2*box2d.LinearSlop || end.At.Y != 0 ||
			math.Abs(end.Until-c.until) > 2*box2d.LinearSlop/0.75 {
			t.Errorf("%s: ended %+v, want at x %v, %v of the tick, on %d", c.name, end, c.at, c.until, c.hit)
		}
	}
}
