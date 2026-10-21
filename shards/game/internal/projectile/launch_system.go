package projectile

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	abilitycomp "echo_image/shards/game/internal/ability/component"
	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/team"
)

// sourceRow is a caster a shot launches from.
type sourceRow struct {
	Team      team.Team
	Transform physics2d.Transform2D
}

// projectileRow is a shot in flight.
type projectileRow struct {
	Projectile component.Projectile
	Team       team.Team
	Transform  physics2d.Transform2D
}

// LaunchSystem (Update, after ability.CastSystem) launches a shot for each cast of an ability that has
// a launcher. O(C) for C casts.
type LaunchSystem struct {
	Data *data.Plugin
}

func (s *LaunchSystem) Run(w *cardinal.World) {
	launchers := data.Get[ProjectileTuning](s.Data).Launchers
	sources := w.Contains[sourceRow]()
	for cast := range w.SystemEvents[abilitycomp.AbilityCast]() {
		l, ok := launchers[cast.Ability]
		if !ok || (cast.AimX == 0 && cast.AimY == 0) {
			continue // not a shot, or no direction to fly in
		}
		source, err := sources.GetByID(cast.Caster)
		if err != nil {
			continue // caster gone this tick
		}
		launch(w, source, l, cast.AimX, cast.AimY)
	}
}

// launch is the one constructor of a shot: it leaves source's position toward the unit vector
// (dirX, dirY), on source's team.
func launch(w *cardinal.World, source cardinal.Entity, l Launcher, dirX, dirY float64) {
	e := w.Create[projectileRow]()
	e.Set(component.Projectile{
		Source:    source.ID(),
		Velocity:  physics2d.Vec2{X: dirX * l.Speed, Y: dirY * l.Speed},
		Radius:    l.Radius,
		Damage:    l.Damage,
		Remaining: l.Range,
	})
	e.Set(source.Get[team.Team]())
	e.Set(physics2d.Transform2D{Position: source.Get[physics2d.Transform2D]().Position})
}
