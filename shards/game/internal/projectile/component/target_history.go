package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/projectile/flight"
)

// TargetHistory is a target's rows from its last ticks, as each tick's syncs showed it: oldest first,
// one per tick. A predicted shot meets it by them, at the view its client drew. Only
// projectile.HistorySystem writes it; each target's constructor (a Body's, an Echo's) adds it.
type TargetHistory struct {
	Rows cardinal.Slice[flight.Row]
}

// Name is the ECS component name.
func (TargetHistory) Name() string { return "target_history" }

// At returns the row saved at tick, if there is one.
func (h TargetHistory) At(tick uint64) (flight.Row, bool) {
	n := h.Rows.Len()
	if n == 0 {
		return flight.Row{}, false
	}
	newest := h.Rows.At(n - 1).Tick
	if tick > newest || newest-tick >= uint64(n) {
		return flight.Row{}, false
	}
	row := h.Rows.At(n - 1 - int(newest-tick))
	return row, row.Tick == tick
}
