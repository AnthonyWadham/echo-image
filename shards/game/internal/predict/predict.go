// Package predict runs the client's own Body ahead of the server: its moves, its casts and their
// cooldowns, the effects they put on it, and the shots it has in flight. It starts from the server's
// state after your last applied frame, and steps each frame the server hasn't applied yet as one tick
// of the server's own code, so with no surprises it lands on the server's bits. What others do
// (effects they put on you, what your shots hit) comes with the next state. It imports no Cardinal:
// shards/game/predictor builds it for the browser.
package predict

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/ability/hooks"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics/timestep"
	"echo_image/shards/game/internal/player/motion"
	"echo_image/shards/game/internal/projectile/flight"
)

// Content is the content the predictor runs: the server's own files, loaded the way the data plugin
// loads them.
type Content struct {
	Abilities cast.Tuning
	Shots     flight.Tuning
	Effects   status.Tuning
}

// Load reads Content from fsys, the content embed.FS: each file decoded, the hooks bound, then each
// checked.
func Load(fsys fs.FS) (Content, error) {
	var c Content
	err := errors.Join(decode(fsys, c.Abilities.JSONFile(), &c.Abilities),
		decode(fsys, c.Shots.JSONFile(), &c.Shots), decode(fsys, c.Effects.JSONFile(), &c.Effects))
	if err == nil {
		err = c.Abilities.Bind()
	}
	if err == nil {
		err = errors.Join(c.Abilities.Validate(), c.Shots.Validate(), c.Effects.Validate())
	}
	return c, err
}

func decode(fsys fs.FS, file string, v any) error {
	raw, err := fs.ReadFile(fsys, file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return nil
}

// State is the server's state of your Body and what it started, after Tick, as body_sync sends it.
type State struct {
	Tick        uint64
	Body        uint32 // your Body's EntityID: the caster, owner and target of what it casts
	Team        uint8
	Center      box2d.Vec2
	Radius      float64
	MoveSpeed   float64
	Champion    string // the champion it plays: its kit
	Cooldowns   cast.Cooldowns
	Effects     []status.Entry
	EffectsNext uint32 // the last serial its effect list gave out
	Shots       []Shot // the shots it cast that are in flight
}

// Shot is one of your shots in flight. Prev is where it was a tick before, for drawing: its launch
// point the tick it launched, and its own spot right after a Reset.
type Shot struct {
	Type string
	flight.Flight
	Prev box2d.Vec2
}

// Frame is one tick of your input as the client sends it: playercomp.InputFrame converts to it. Seq
// isn't read.
type Frame struct {
	Seq          uint32
	MoveX, MoveY float64
	AimX, AimY   float64
	Buttons      uint32
}

// Predictor steps your Body one tick per frame from the last State. Its world has no map yet; walls
// come with the arena.
type Predictor struct {
	content Content
	world   *box2d.World
	state   State
}

// New returns a Predictor that runs content. Reset it before stepping.
func New(content Content) *Predictor {
	def := box2d.DefaultWorldDef()
	return &Predictor{content: content, world: box2d.NewWorld(&def)}
}

// Reset starts over from s.
func (p *Predictor) Reset(s State) {
	s.Cooldowns, s.Effects, s.Shots = slices.Clone(s.Cooldowns), slices.Clone(s.Effects), slices.Clone(s.Shots)
	for i := range s.Shots {
		s.Shots[i].Prev = s.Shots[i].Pos
	}
	p.state = s
}

// State returns the state after the last step.
func (p *Predictor) State() State {
	s := p.state
	s.Cooldowns, s.Effects, s.Shots = slices.Clone(s.Cooldowns), slices.Clone(s.Effects), slices.Clone(s.Shots)
	return s
}

// Kit returns when each ability in your kit can be cast again, slot 0 first: 0 if it never was.
func (p *Predictor) Kit() []cast.Cooldown {
	s := &p.state
	kit := p.content.Abilities.Kits[s.Champion]
	ready := make([]cast.Cooldown, len(kit))
	for i, id := range kit {
		ready[i] = cast.Cooldown{Ability: id, ReadyTick: s.Cooldowns.ReadyTick(id)}
	}
	return ready
}

// Peek returns where a frame moving (moveX, moveY) would leave your Body after the next tick, without
// stepping. The client aims from there, since the server casts from there.
func (p *Predictor) Peek(moveX, moveY float64) box2d.Vec2 {
	s := &p.state
	return p.move(moveX, moveY, status.Fold(slices.Values(s.Effects), s.Tick+1))
}

// Step runs the next tick for frame f, in the server's order: the cast gate, the move, each cast's
// hook from where the move left your Body, its shots, then its effects on you from the tick after. It
// returns your Body's new center.
func (p *Predictor) Step(f Frame) box2d.Vec2 {
	s := &p.state
	s.Tick++
	st := status.Fold(slices.Values(s.Effects), s.Tick)
	aimX, aimY := motion.Unit(f.AimX, f.AimY)
	aim := hooks.Vec2{X: aimX, Y: aimY}
	var casts []string
	s.Cooldowns, casts = p.content.Abilities.Gate(s.Champion, s.Cooldowns, st.Tags, f.Buttons, aim, s.Tick)
	s.Center = p.move(f.MoveX, f.MoveY, st)
	var out hooks.Out
	for _, id := range casts {
		if hook, ok := p.content.Abilities.Hook(id); ok && hook.OnCast != nil {
			hook.OnCast(hooks.Cast{
				Caster: s.Body, Owner: s.Body, Team: s.Team, // a player Body acts for itself
				Pos: hooks.Vec2(s.Center), Aim: aim, TickRate: timestep.Rate,
			}, &out)
		}
	}
	p.fly(out.Launches)
	p.affect(out.Effects)
	return s.Center
}

// move is where moving (moveX, moveY) under st leaves your Body after one tick.
func (p *Predictor) move(moveX, moveY float64, st status.Status) box2d.Vec2 {
	s := &p.state
	x, y := motion.Unit(moveX, moveY)
	center, _ := motion.Step(p.world, motion.Body{
		Center: s.Center,
		Radius: s.Radius,
		Move:   box2d.Vec2{X: x, Y: y},
		Speed:  s.MoveSpeed,
	}, st, timestep.Seconds)
	return center
}

// fly launches this tick's shots from your Body, then flies all your shots one tick past nothing:
// what they hit is the server's to say, and comes with the next state.
func (p *Predictor) fly(launches []hooks.Launch) {
	s := &p.state
	shots := p.content.Shots
	for _, l := range launches {
		if def, ok := shots.Shots[l.Shot]; ok {
			s.Shots = append(s.Shots, Shot{Type: l.Shot, Flight: flight.Launch(def, s.Center, box2d.Vec2(l.Dir))})
		}
	}
	side := interact.Side{Owner: s.Body, Team: s.Team}
	kept := s.Shots[:0]
	for _, shot := range s.Shots {
		def, ok := shots.Shots[shot.Type]
		if !ok {
			continue
		}
		next, over, _ := shots.Fly(p.world, def, shot.Flight, side, nil, timestep.Seconds)
		if over {
			continue
		}
		shot.Prev, shot.Flight = shot.Pos, next
		kept = append(kept, shot)
	}
	s.Shots = kept
}

// affect adds the effects your casts put on your own Body, from the next tick, and drops the ones
// over by then, as effect.ResolveSystem does. Effects on others are theirs to predict.
func (p *Predictor) affect(effects []hooks.Effect) {
	s := &p.state
	rules := p.content.Effects
	var mine []status.Intent
	for _, e := range effects {
		if e.Target == s.Body {
			mine = append(mine, cast.EffectIntent(e, s.Body))
		}
	}
	slices.SortFunc(mine, rules.Compare)
	next := s.Tick + 1
	for _, in := range mine {
		s.Effects, _ = rules.Apply(s.Effects, &s.EffectsNext, in, next)
	}
	s.Effects = status.Sweep(s.Effects, next)
}
