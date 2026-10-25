package projectile

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/data"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/team"
)

// projectileRow is a shot in flight.
type projectileRow struct {
	Projectile component.Projectile
	Team       team.Team
	Owner      team.Owner
	Transform  physics2d.Transform2D
}

// LaunchSystem (Update, after every LaunchIntent sender, before FlightSystem) launches a shot for each
// LaunchIntent, in sorted order, since emit order can change after a restore and the order sets the
// shots' entity IDs. O(L log L) for L launches.
type LaunchSystem struct {
	Data *data.Plugin
}

func (s *LaunchSystem) Run(w *cardinal.World) {
	shots := data.Get[ProjectileTuning](s.Data).Shots
	for _, in := range slices.SortedFunc(w.SystemEvents[component.LaunchIntent](), compareLaunch) {
		def, ok := shots[in.Shot]
		if !ok {
			w.Logger().Error().Str("shot", in.Shot).Msg("launched no shot of unknown type")
			continue
		}
		launch(w, in, def)
	}
}

// launch is the one constructor of a shot: a shot of type in.Shot leaves in.From along in.Dir, for
// in.Owner on in.Team.
func launch(w *cardinal.World, in component.LaunchIntent, def Shot) {
	e := w.Create[projectileRow]()
	e.Set(component.Projectile{
		Type:      in.Shot,
		Cause:     in.Cause,
		Velocity:  physics2d.Vec2{X: in.Dir.X * def.Speed, Y: in.Dir.Y * def.Speed},
		Radius:    def.Radius,
		Damage:    def.Damage,
		Remaining: def.Range,
	})
	e.Set(team.Team{ID: in.Team})
	e.Set(team.Owner{Body: in.Owner})
	e.Set(physics2d.Transform2D{Position: in.From})
}

func compareLaunch(a, b component.LaunchIntent) int {
	return cmp.Or(
		cmp.Compare(a.Cause.Caster, b.Cause.Caster),
		cmp.Compare(a.Cause.Ability, b.Cause.Ability),
		cmp.Compare(a.Cause.Tick, b.Cause.Tick),
		cmp.Compare(a.Shot, b.Shot),
		cmp.Compare(a.Owner, b.Owner),
		cmp.Compare(a.Team, b.Team),
		cmp.Compare(a.From.X, b.From.X),
		cmp.Compare(a.From.Y, b.From.Y),
		cmp.Compare(a.Dir.X, b.Dir.X),
		cmp.Compare(a.Dir.Y, b.Dir.Y),
	)
}
