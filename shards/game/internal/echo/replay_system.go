package echo

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/echo/component"
	playercomp "echo_image/shards/game/internal/player/component"
)

// replayRow is an Echo we drive: its identity + trail, and the Transform the replay writes each tick.
type replayRow struct {
	Echo      cardinal.Ref[component.Echo]
	History   cardinal.Ref[component.PathHistory]
	Transform cardinal.Ref[physics2d.Transform2D]
}

// ownerPoseRow reads a Body's current pose by id (an Echo's Owner). Player qualifies the match to
// Bodies only, so GetByID errors cleanly if Owner's id was recycled for a non-Body entity instead of
// silently returning an unrelated entity's pose. Player is matched on, never read.
type ownerPoseRow struct {
	Player    cardinal.Ref[playercomp.Player]
	Transform cardinal.Ref[physics2d.Transform2D]
}

// ReplaySystemState records each Body's pose into its Echo's ring and drives the Echo from the delayed
// slot.
type ReplaySystemState struct {
	cardinal.BaseSystemState
	Echoes cardinal.Contains[replayRow]
	Bodies cardinal.Contains[ownerPoseRow]
}

// ReplaySystem (Update, after the physics PreUpdate step so the Body pose is current) samples each
// Body's pose into its Echo's ring buffer and moves the Echo to the pose from delayTicks ago — the
// delayed replay. The Echo is BodyTypeManual, so writing its Transform teleports the Box2D body on the
// next reconcile.
//
// It is the SOLE writer of PathHistory, so the Get→mutate→Set on that aliasing slice stays inside one
// system (see component.PathHistory). It also counts Warmup down; anti-camp reads Warmup to stay off
// until the trail holds real history.
func ReplaySystem(state *ReplaySystemState) {
	for _, r := range state.Echoes.Iter() {
		ec := r.Echo.Get()
		body, err := state.Bodies.GetByID(ec.Owner)
		if err != nil {
			continue // owner Body gone
		}

		h := r.History.Get()
		n := len(h.Poses) // >= 2: EchoTuning.Validate forces DelayTicks >= 1 and spawn sizes the ring to delay+1
		// Record the Body's current pose as the newest frame, then read the oldest slot — the pose from
		// delayTicks ago, since the ring is delayTicks+1 long — and drive the Echo there.
		h.Head = (h.Head + 1) % n
		h.Poses[h.Head] = component.EchoFrame{Pose: body.Transform.Get()}
		oldest := (h.Head + 1) % n
		r.Transform.Set(h.Poses[oldest].Pose)
		r.History.Set(h)

		if ec.Warmup > 0 {
			ec.Warmup--
			r.Echo.Set(ec)
		}
	}
}
