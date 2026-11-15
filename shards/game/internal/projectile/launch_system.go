package projectile

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d"

	"echo_image/shards/game/internal/projectile/component"
	"echo_image/shards/game/internal/projectile/flight"
	"echo_image/shards/game/internal/ref"
	"echo_image/shards/game/internal/team"
)

// projectileRow is a shot in flight.
type projectileRow struct {
	Spawn      ref.Spawn
	Projectile component.Projectile
	Team       team.Team
	Owner      team.Owner
	Transform  physics2d.Transform2D
}

// LaunchSystem (Update, after every LaunchIntent sender, before FlightSystem) launches a shot for each
// LaunchIntent, in sorted order, since emit order can change after a restore and the order sets the
// shots' entity IDs. O(L log L) for L launches.
type LaunchSystem struct {
	Tuning ProjectileTuning
}

func (s *LaunchSystem) Run(w *cardinal.World) {
	shots := s.Tuning.Shots
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
// in.Owner on in.Team. Its flight starts as flight.Launch, which the client predictor launches with. A
// shot an input frame cast is predicted by its caster's client.
func launch(w *cardinal.World, in component.LaunchIntent, def flight.Shot) {
	f := flight.Launch(def, box2d.Vec2(in.From), box2d.Vec2(in.Dir))
	e := w.Create[projectileRow]()
	e.Set(ref.Spawn{Serial: ref.Next(w)})
	e.Set(component.Projectile{
		Type:      in.Shot,
		Cause:     in.Cause,
		Launch:    in.Launch,
		Predicted: in.Cause.Seq != 0,
		Velocity:  physics2d.Vec2(f.Velocity),
		Radius:    f.Radius,
		Damage:    def.Damage,
		Remaining: f.Remaining,
	})
	e.Set(team.Team{ID: in.Team})
	e.Set(team.Owner{Body: in.Owner})
	e.Set(physics2d.Transform2D{Position: physics2d.Vec2(f.Pos)})
}

func compareLaunch(a, b component.LaunchIntent) int {
	return cmp.Or(
		a.Cause.Compare(b.Cause),
		cmp.Compare(a.Launch, b.Launch),
		cmp.Compare(a.Shot, b.Shot),
		cmp.Compare(a.Owner, b.Owner),
		cmp.Compare(a.Team, b.Team),
		cmp.Compare(a.From.X, b.From.X),
		cmp.Compare(a.From.Y, b.From.Y),
		cmp.Compare(a.Dir.X, b.Dir.X),
		cmp.Compare(a.Dir.Y, b.Dir.Y),
	)
}
