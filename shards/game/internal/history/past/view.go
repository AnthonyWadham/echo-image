// Package past is the Cardinal-free core of lag compensation: a thing's row, the view rule that draws
// and judges it, and the window a view may lag. The shard and the client predictor (internal/predict)
// run the same code. It imports only box2d, interact, physics/timestep and the standard library, so it
// builds for wasm.
package past

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/interact"
)

// Row is a target as one tick's syncs show it: what it is, whose, where, its size, and the tags its
// effects give it from the next tick on, as that tick's rules read them. It is the one record of a
// target: its History keeps its rows, and the target sync sends each tick's.
type Row struct {
	Tick   uint64
	Kind   interact.Kind
	Side   interact.Side
	Center box2d.Vec2
	Radius float64
	Tags   interact.Tags
	Until  float64 // the part of the tick it lived: 1, or when it died in it
}

// RowTicks is the ticks of the rows view v is drawn from: b - 1 and b = ⌈v⌉. v is t = v - (b - 1) of the
// way through tick b, 0 < t <= 1, so a whole tick is its end. The client's Timeline.sample picks the
// same two.
func RowTicks(view float64) (older, newer uint64) {
	b := uint64(math.Ceil(view))
	return b - 1, b
}

// AtView is a target where view v showed it, from its rows at RowTicks(v), or false if it showed none.
// It is newer, its row at b, drawn partway from older, its row at b - 1: t / newer.Until of the way, and
// gone past newer.Until. With no row at b - 1 (it is new at b), it is newer as it is. This is the rule
// the client draws by (EntityViews' blend); testdata/view.json pins the two together.
func AtView(older Row, hasOlder bool, newer Row, view float64) (Row, bool) {
	t := view - (math.Ceil(view) - 1)
	if t > newer.Until {
		return Row{}, false
	}
	newer.Center = along(older, hasOlder, newer, t)
	return newer, true
}

// along is where a target drawn from its rows older to newer is t of the way through newer's tick, t at
// most newer.Until: partway from older's center at its own pace, or at newer's with no older row.
func along(older Row, hasOlder bool, newer Row, t float64) box2d.Vec2 {
	if !hasOlder {
		return newer.Center
	}
	return lerp(older.Center, newer.Center, t/newer.Until)
}

// lerp is k of the way from a to b.
func lerp(a, b box2d.Vec2, k float64) box2d.Vec2 {
	return box2d.Vec2{X: a.X + float64((b.X-a.X)*k), Y: a.Y + float64((b.Y-a.Y)*k)}
}

// Span is part of a shot's tick, Start to End (0 to 1), in which a target shows as Row, moving from
// Row.Center to To at its own pace. One with End at most Start is empty.
type Span struct {
	Row
	Start, End float64
	To         box2d.Vec2
}

// Across is a target as views v to v + 1 show it, from row, its row at each tick: those views are what
// a shot's tick judged at view v is drawn across, since a frame's shots fly a tick while the view moves
// a tick. It has a span for each pair of rows those views are drawn from that shows it, from where
// AtView places it as the span starts to where as it ends.
func Across(v float64, row func(tick uint64) (Row, bool)) [2]Span {
	var spans [2]Span
	first := uint64(math.Floor(v)) + 1 // the newer row of the first pair
	for i := range spans {
		b := first + uint64(i)
		newer, ok := row(b)
		if !ok {
			continue
		}
		older, hasOlder := row(b - 1)
		d := float64(b-1) - v // when in the shot's tick b's views begin
		start, end := max(d, 0), min(d+newer.Until, 1)
		if !(end > start) {
			continue
		}
		spans[i] = Span{Row: newer, Start: start, End: end, To: along(older, hasOlder, newer, end-d)}
		spans[i].Center = along(older, hasOlder, newer, start-d)
	}
	return spans
}
