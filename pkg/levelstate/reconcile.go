package levelstate

import (
	"errors"
	"fmt"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// currentRow is the entity that holds Current.
type currentRow struct {
	Current Current
}

// reconcileSystem makes the map state names live. It takes a requested change, then the build's file for
// that map, raising the epoch if either is new, and swaps memory to it.
type reconcileSystem struct {
	plugin *Plugin
	// emit is false at Init, whose system events would reach the first tick even after a restore.
	emit bool
}

// Run panics if state names a map the build lacks: the snapshot came from another build, and a world purge
// starts over. State is empty at Init, so that only happens in a tick, where a purge can clear it.
func (s *reconcileSystem) Run(w *cardinal.World) {
	e := current(w)
	was := e.Get[Current]()
	now := was
	if now.Next != "" {
		now.ID, now.Next = now.Next, ""
	}
	if now.ID == "" {
		now.ID = s.plugin.defaultID
	}
	live, ok := s.plugin.maps[now.ID]
	if !ok {
		panic(fmt.Errorf("levelstate: map %q isn't in this build; run world purge", now.ID))
	}
	if now.ID != was.ID || now.Hash != live.hash {
		now.Hash = live.hash
		now.Epoch++
	}
	if now != was {
		e.Set(now)
	}
	s.plugin.live = live
	if s.emit && now.Epoch != was.Epoch {
		w.EmitSystemEvent(Changed{Previous: was.ID, ID: now.ID, Epoch: now.Epoch})
	}
}

// current returns the entity that holds Current, made with a zero Current if none does.
func current(w *cardinal.World) cardinal.Entity {
	search := w.Contains[currentRow]()
	e, err := search.Iter().Single()
	if errors.Is(err, cardinal.ErrSingleNoResult) {
		return search.Create()
	}
	if err != nil {
		panic(fmt.Errorf("levelstate: %w", err))
	}
	return e
}
