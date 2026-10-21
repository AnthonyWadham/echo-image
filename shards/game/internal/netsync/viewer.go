package netsync

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/team"
)

// Viewer is a player who controls a Body, and so receives game state each tick.
type Viewer struct {
	Player string          // player ID, the SendTo recipient
	Team   uint8           // allies share vision, so lists are built per team
	Body   cardinal.Entity // the viewer's own Body
}

type viewerRow struct {
	Player playercomp.Player
	Team   team.Team
}

// Viewers returns this tick's viewers. O(B).
func Viewers(w *cardinal.World) []Viewer {
	var viewers []Viewer
	for e := range w.Contains[viewerRow]().Iter() {
		if p := e.Get[playercomp.Player](); p.Persona != "" {
			viewers = append(viewers, Viewer{Player: p.Persona, Team: e.Get[team.Team]().ID, Body: e})
		}
	}
	return viewers
}
