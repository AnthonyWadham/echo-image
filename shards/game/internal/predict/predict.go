// Package predict runs the client's own Body ahead of the server: its moves, its casts and their
// cooldowns, the effects they put on it, the shots it has in flight, and its Echo. It starts from the server's
// state after your last applied frame, and steps each frame the server hasn't applied yet as one tick
// of the server's own code, so with no surprises it lands on the server's bits. Your shots meet each
// target (a Body or Echo) where your screen drew it, as the server's FlightSystem judges them. What others
// do (effects they put on you, kills) comes with the next state. It imports no Cardinal:
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
	EchoDelay   uint32 // ticks its Echo trails it: 0 with no Echo
	// EchoPath is its Echo's spot at Tick, then where it will be on each next tick, as far as the server
	// sent.
	EchoPath []box2d.Vec2
}

// Shot is one of your shots in flight. Prev is where its flight in the newest tick began, for drawing:
// its launch point the tick it launched.
type Shot struct {
	Type string
	Key
	flight.Flight
	Prev box2d.Vec2
}

// Ended is one of your shots that ended in a tick: the shot as the tick found it, and how it ended.
type Ended struct {
	Shot
	End flight.End
}

// Key is a shot's key: the input frame and ability that cast it, and its place among that cast's
// launches. The client knows it for sure, while a frame's tick shifts when it arrives late.
type Key struct {
	Seq     uint32
	Ability string
	Launch  uint32
}

// Frame is one tick of your input as the client sends it, as playercomp.InputFrame. Seq keys the shots
// it casts, and ViewTick is the render tick on screen.
type Frame struct {
	Seq          uint32
	MoveX, MoveY float64
	AimX, AimY   float64
	Buttons      uint32
	ViewTick     float64
}

// Sight is a target as a target sync showed it: its entity and serial, and its row. The predictor keeps
// what it saw by Serial, since a newer entity can take ID.
type Sight struct {
	ID     uint32
	Serial uint64
	flight.Row
}

// Predictor steps your Body one tick per frame from the last State. Its world has no map yet; walls
// come with the arena. It holds a body for each target alive in the newest target sync, placed where
// each frame's view drew it.
type Predictor struct {
	content Content
	world   *box2d.World
	state   State
	start   uint64                      // the state's tick at the last Reset
	moves   []box2d.Vec2                // your Body's center after each step since then
	seen    map[uint64]map[uint64]Sight // the last KeptRows ticks of target syncs: by tick, then serial
	targets map[uint64]target           // by serial
}

// target is a target's body in the predictor's world. Its user data is the entity's serial.
type target struct {
	body   box2d.BodyID
	kind   interact.Kind
	radius float64
}

// New returns a Predictor that runs content. Reset it before stepping.
func New(content Content) *Predictor {
	def := box2d.DefaultWorldDef()
	return &Predictor{
		content: content, world: box2d.NewWorld(&def), seen: map[uint64]map[uint64]Sight{},
		targets: map[uint64]target{},
	}
}

// See keeps what the target sync of tick showed, so a frame can place each target where its view drew
// it. The newest sync says which targets have a body: those that lived its whole tick, since the server
// removes one that died at the end of its tick, before any later frame's shots fly.
func (p *Predictor) See(tick uint64, sights []Sight) {
	at := make(map[uint64]Sight, len(sights))
	for _, s := range sights {
		s.Tick = tick
		at[s.Serial] = s
	}
	p.seen[tick] = at
	keep := p.content.Shots.KeptRows()
	for t := range p.seen {
		if t+keep <= tick {
			delete(p.seen, t)
		}
	}
	for t := range p.seen {
		if t > tick {
			return // a newer sync said which have a body
		}
	}
	for _, s := range sights {
		if s.Until >= 1 {
			p.ensure(s.Serial, s.Kind, s.Radius)
		}
	}
	for serial, t := range p.targets {
		if s, ok := at[serial]; !ok || s.Until < 1 {
			p.world.DestroyBody(t.body)
			delete(p.targets, serial)
		}
	}
}

// Forget drops every sync it saw and every target's body: after a reconnect or a server restart.
func (p *Predictor) Forget() {
	for _, t := range p.targets {
		p.world.DestroyBody(t.body)
	}
	clear(p.targets)
	clear(p.seen)
}

// ensure gives the entity with serial a body of kind with one circle of radius at its origin, as the
// server's targets have. Its filter meets only shots.
func (p *Predictor) ensure(serial uint64, kind interact.Kind, radius float64) target {
	t, ok := p.targets[serial]
	if ok && t.kind == kind && t.radius == radius {
		return t
	}
	if ok {
		p.world.DestroyBody(t.body)
	}
	def := box2d.DefaultBodyDef()
	def.Type = box2d.KinematicBody
	body := p.world.CreateBody(&def)
	p.world.SetBodyUserData(body, serial)
	shape := box2d.DefaultShapeDef()
	shape.Filter = box2d.Filter{CategoryBits: uint64(kind), MaskBits: uint64(interact.Shot)}
	p.world.CreateCircleShape(body, &shape, &box2d.Circle{Radius: radius})
	t = target{body: body, kind: kind, radius: radius}
	p.targets[serial] = t
	return t
}

// place puts each target where view drew it (flight.AtView), held to the rows the server keeps
// (flight.Tuning.View), your Body where it is now with your tags st, and your Echo where it is now
// (Echo), as FlightSystem does. It returns whose each placed shape is, or nil when the target syncs of
// that view's two ticks aren't both here: then your shots meet no target.
func (p *Predictor) place(view float64, st status.Status) flight.Targets {
	s := &p.state
	view = p.content.Shots.View(view, s.Tick)
	a, b := flight.RowTicks(view)
	older, hasOlderSync := p.seen[a]
	newer, hasNewerSync := p.seen[b]
	if !hasOlderSync || !hasNewerSync {
		return nil
	}
	echo, _, hasEcho := p.Echo()
	placed := map[uint64]flight.Target{}
	for serial := range p.targets {
		n, ok := newer[serial]
		if !ok {
			continue
		}
		if n.ID == s.Body {
			p.world.SetBodyTransform(p.ensure(serial, n.Kind, s.Radius).body, s.Center, box2d.RotIdentity)
			side := interact.Side{Owner: s.Body, Team: s.Team}
			placed[serial] = flight.Target{ID: s.Body, Serial: serial, Side: side, Tags: st.Tags}
			continue
		}
		o, hasOlder := older[serial]
		row, ok := flight.AtView(o.Row, hasOlder, n.Row, view)
		if !ok {
			continue
		}
		if hasEcho && row.Kind == interact.Echo && row.Side.Owner == s.Body {
			row.Center = echo
		}
		p.world.SetBodyTransform(p.ensure(serial, row.Kind, row.Radius).body, row.Center, box2d.RotIdentity)
		placed[serial] = flight.Target{ID: n.ID, Serial: serial, Side: row.Side, Tags: row.Tags}
	}
	return func(shape box2d.ShapeID) (flight.Target, bool) {
		t, ok := placed[p.world.BodyUserData(p.world.ShapeBody(shape))]
		return t, ok
	}
}

// Reset starts over from s.
func (p *Predictor) Reset(s State) {
	s.Cooldowns, s.Effects, s.Shots = slices.Clone(s.Cooldowns), slices.Clone(s.Effects), slices.Clone(s.Shots)
	s.EchoPath = slices.Clone(s.EchoPath)
	p.state, p.start, p.moves = s, s.Tick, p.moves[:0]
}

// State returns the state after the last step.
func (p *Predictor) State() State {
	s := p.state
	s.Cooldowns, s.Effects, s.Shots = slices.Clone(s.Cooldowns), slices.Clone(s.Effects), slices.Clone(s.Shots)
	s.EchoPath = slices.Clone(s.EchoPath)
	return s
}

// Echo returns where your Echo is after the newest step, and where it was a tick before, or false with
// no Echo. It retraces your Body EchoDelay ticks behind, as echo.ReplaySystem moves it: along the path
// the state sent, then where your predicted Body was. Past the path and short of that, it waits at the
// path's end.
func (p *Predictor) Echo() (at, prev box2d.Vec2, ok bool) {
	if len(p.state.EchoPath) == 0 {
		return box2d.Vec2{}, box2d.Vec2{}, false
	}
	k := int(p.state.Tick - p.start)
	return p.echoAt(k), p.echoAt(max(k-1, 0)), true
}

// echoAt is your Echo's spot k ticks after the last Reset.
func (p *Predictor) echoAt(k int) box2d.Vec2 {
	path, delay := p.state.EchoPath, int(p.state.EchoDelay)
	switch {
	case k < len(path):
		return path[k]
	case k > delay:
		return p.moves[k-delay-1] // where your Body was after step k - delay
	}
	return path[len(path)-1]
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
// hook from where the move left your Body, its shots, then its effects on you from the tick after. Your
// shots meet the Bodies and Echoes where f's view drew them, and a shot that stops on one ends there.
// It returns your Body's new center and the shots that ended in the tick. A frame replayed after a Reset
// steps the same way.
func (p *Predictor) Step(f Frame) (box2d.Vec2, []Ended) {
	s := &p.state
	s.Tick++
	st := status.Fold(slices.Values(s.Effects), s.Tick)
	aimX, aimY := motion.Unit(f.AimX, f.AimY)
	aim := hooks.Vec2{X: aimX, Y: aimY}
	var casts []string
	s.Cooldowns, casts = p.content.Abilities.Gate(s.Champion, s.Cooldowns, st.Tags, f.Buttons, aim, s.Tick)
	s.Center = p.move(f.MoveX, f.MoveY, st)
	p.moves = append(p.moves, s.Center)
	asked := asks{body: s.Body}
	for _, id := range casts {
		p.content.Abilities.Run(id, hooks.Cast{
			Caster: s.Body, Owner: s.Body, Team: s.Team, // a player Body acts for itself
			Pos: hooks.Vec2(s.Center), Aim: aim,
		}, &asked)
	}
	ends := p.fly(asked.launches, f.Seq, p.place(f.ViewTick, st))
	p.affect(asked.effects)
	return s.Center, ends
}

// asks is a cast.Sink for one frame's casts: your shots, and the effects on your own Body. Effects on
// others are theirs to predict.
type asks struct {
	body     uint32
	launches []cast.Launch
	effects  []status.Intent
}

func (a *asks) Launch(l cast.Launch) { a.launches = append(a.launches, l) }

func (a *asks) Effect(in status.Intent) {
	if in.Target == a.body {
		a.effects = append(a.effects, in)
	}
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

// fly launches the shots frame seq's casts asked for, then flies all your shots one tick. One that stops
// on a Body or Echo of targets ends there; with no targets, shots pass Bodies and Echoes. It returns the
// shots that ended.
func (p *Predictor) fly(launches []cast.Launch, seq uint32, targets flight.Targets) []Ended {
	s := &p.state
	shots := p.content.Shots
	for _, l := range launches {
		if def, ok := shots.Shots[l.Shot]; ok {
			key := Key{Seq: seq, Ability: l.Ability, Launch: l.Launch}
			s.Shots = append(s.Shots, Shot{Type: l.Shot, Key: key,
				Flight: flight.Launch(def, box2d.Vec2(l.From), box2d.Vec2(l.Dir))})
		}
	}
	side := interact.Side{Owner: s.Body, Team: s.Team}
	var ends []Ended
	kept := s.Shots[:0]
	for _, shot := range s.Shots {
		def, ok := shots.Shots[shot.Type]
		if !ok {
			continue
		}
		next, end, over := shots.Fly(p.world, def, shot.Flight, side, targets, timestep.Seconds)
		if over {
			ends = append(ends, Ended{Shot: shot, End: end})
			continue
		}
		shot.Prev, shot.Flight = shot.Pos, next
		kept = append(kept, shot)
	}
	s.Shots = kept
	return ends
}

// affect adds the effects your casts put on your own Body, from the next tick, and drops the ones
// over by then, as effect.ResolveSystem does.
func (p *Predictor) affect(effects []status.Intent) {
	s := &p.state
	rules := p.content.Effects
	slices.SortFunc(effects, rules.Compare)
	next := s.Tick + 1
	for _, in := range effects {
		s.Effects, _ = rules.Apply(s.Effects, &s.EffectsNext, in, next)
	}
	s.Effects = status.Sweep(s.Effects, next)
}
