package combat

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	echocomp "echo_image/shards/game/internal/echo/component"
	"echo_image/shards/game/internal/physics"
	playercomp "echo_image/shards/game/internal/player/component"
)

// campingEchoRow is an Echo we test for the anti-camp overlap: its identity (Owner + Warmup) and pose.
type campingEchoRow struct {
	Echo      echocomp.Echo
	Transform physics2d.Transform2D
}

// bodyPoseRow reads an Echo owner's Body pose by id. Player qualifies the match to Bodies only, so
// GetByID errors cleanly if Owner's id was recycled for a non-Body entity. Player is matched, never read.
type bodyPoseRow struct {
	Player    playercomp.Player
	Transform physics2d.Transform2D
}

// AntiCampSystem (GDD §3): an Echo overlapping its OWN Body self-damages, so you can never stand still.
// It's a per-tick distance check, not a physics contact — an Echo (BodyTypeManual) and Body (kinematic)
// generate no solid contact — that emits a DamageIntent, keeping DamageResolveSystem the single writer
// of Health. The overlap is an OWNER match (the Echo vs its own Owner Body), never a team match.
//
// Self-damage is continuous, so it's per-SECOND scaled by dt (tick-rate independent). An Echo still
// warming up (Warmup > 0) is skipped: for the first delay window its trail replays the prefilled spawn
// pose, so it sits on its Body at match start — this grace is what stops the spawn overlap from
// instantly killing it.
type AntiCampSystem struct {
	Data *data.Plugin
}

func (s *AntiCampSystem) Run(w *cardinal.World) {
	tuning := data.Get[CombatTuning](s.Data) // once at entry, not per Echo
	perTick := tuning.SelfDamagePerSecond * physics.SecondsPerTick
	if perTick <= 0 {
		return // anti-camp disabled by tuning
	}
	overlapSq := tuning.OverlapRadius * tuning.OverlapRadius

	bodies := w.Contains[bodyPoseRow]()
	for e := range w.Contains[campingEchoRow]().Iter() {
		ec := e.Get[echocomp.Echo]()
		if ec.Warmup > 0 {
			continue // trail not yet real; spare the start-of-match spawn overlap
		}
		body, err := bodies.GetByID(ec.Owner)
		if err != nil {
			continue // owner Body gone
		}
		ep := e.Get[physics2d.Transform2D]().Position
		bp := body.Get[physics2d.Transform2D]().Position
		dx, dy := ep.X-bp.X, ep.Y-bp.Y
		if dx*dx+dy*dy < overlapSq {
			w.EmitSystemEvent(DamageIntent{Target: e.ID(), Source: ec.Owner, Amount: perTick})
		}
	}
}
