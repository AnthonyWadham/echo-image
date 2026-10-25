package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/echo/component"
	playercomp "echo_image/shards/game/internal/player/component"
	"echo_image/shards/game/internal/team"
)

// replayRow is an Echo we drive: its identity + trail, and the Transform the replay writes each tick.
type replayRow struct {
	Echo      component.Echo
	Owner     team.Owner
	History   component.PathHistory
	Transform physics2d.Transform2D
}

// ownerPoseRow reads a Body's current pose by id (an Echo's Owner). Player qualifies the match to
// Bodies only, so GetByID errors cleanly if Owner's id was recycled for a non-Body entity instead of
// silently returning an unrelated entity's pose. Player is matched on, never read.
type ownerPoseRow struct {
	Player    playercomp.Player
	Transform physics2d.Transform2D
}

// ReplaySystem (Update, after the physics PreUpdate step so the Body pose is current) samples each
// Body's pose into its Echo's ring buffer and moves the Echo to the pose from delayTicks ago — the
// delayed replay. Each Echo's ring is sized to its champion's delay at spawn. The Echo is BodyTypeManual, so writing its Transform teleports the Box2D body on the
// next reconcile.
//
// It is the SOLE writer of PathHistory, so the Get→mutate→Set on that aliasing slice stays inside one
// system (see component.PathHistory).
type ReplaySystem struct{}

func (s *ReplaySystem) Run(w *cardinal.World) {
	bodies := w.Contains[ownerPoseRow]()
	for r := range w.Contains[replayRow]().Iter() {
		body, err := bodies.GetByID(r.Get[team.Owner]().Body)
		if err != nil {
			continue // owner Body gone
		}

		h := r.Get[component.PathHistory]()
		n := h.Poses.Len() // >= 2: EchoTuning.Validate forces each delay >= 1 tick and spawn sizes the ring to delay+1
		// Record the Body's current pose as the newest frame, then read the oldest slot — the pose from
		// delayTicks ago, since the ring is delayTicks+1 long — and drive the Echo there.
		h.Head = (h.Head + 1) % n
		h.Poses = h.Poses.With(h.Head, component.EchoFrame{Pose: body.Get[physics2d.Transform2D]()})
		oldest := (h.Head + 1) % n
		r.Set(h.Poses.At(oldest).Pose)
		r.Set(h)
	}
}
