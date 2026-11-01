// Package predict runs the client's own Body ahead of the server: its moves, its casts and their
// cooldowns, the effects they put on it, and the shots it has in flight. It starts from the server's
// state after your last applied frame, and steps each frame the server hasn't applied yet as one tick
// of the server's own code, so with no surprises it lands on the server's bits. Your shots meet the
// Bodies and Echoes where your screen drew them, and a stop on one is a hit claim the server checks.
// What others do (effects they put on you) comes with the next state. It imports no Cardinal:
// shards/game/predictor builds it for the browser.
package predict

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
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
// it casts, ViewTick is the render tick on screen, and Claims are the shots its step ended.
type Frame struct {
	Seq          uint32
	MoveX, MoveY float64
	AimX, AimY   float64
	Buttons      uint32
	ViewTick     float64
	Claims       []Claim
}

// Claim says a frame's step ended your shot Key on the Body or Echo Target.
type Claim struct {
	Key
	Target uint32
}

// Sight is a Body or Echo as one sync showed it. A Body's owner is itself.
type Sight struct {
	ID     uint32
	Side   interact.Side
	Center box2d.Vec2
	Radius float64
	Tags   interact.Tags
	Until  float64 // the part of the tick it lived: 1, or when it died in it
}

// Predictor steps your Body one tick per frame from the last State. Its world has no map yet; walls
// come with the arena. It holds a body for your Body and for each Body and Echo the newest syncs show,
// placed where each frame's view drew them.
type Predictor struct {
	content Content
	world   *box2d.World
	state   State
	seen    map[uint64]*seen         // the last RewindTicks+1 ticks of syncs, by tick
	newest  map[interact.Kind]uint64 // the newest sync's tick, by kind
	targets map[uint32]target        // by entity ID
}

// seen is what one tick's syncs showed: whose syncs came (Body, Echo or both), and each Body and Echo.
type seen struct {
	synced interact.Kind
	sights map[uint32]sight
}

type sight struct {
	side interact.Side
	row  flight.Row
}

// target is a Body's or Echo's body in the predictor's world. Its user data is the entity ID.
type target struct {
	body   box2d.BodyID
	kind   interact.Kind
	radius float64
}

// New returns a Predictor that runs content. Reset it before stepping.
func New(content Content) *Predictor {
	def := box2d.DefaultWorldDef()
	return &Predictor{
		content: content, world: box2d.NewWorld(&def),
		seen: map[uint64]*seen{}, newest: map[interact.Kind]uint64{}, targets: map[uint32]target{},
	}
}

// See keeps what a sync at tick showed of the Bodies (kind interact.Body, from body_sync) or the Echoes
// (interact.Echo, from echo_sync), so a frame can place them where its view drew them. The newest sync
// of a kind says which of that kind have a body.
func (p *Predictor) See(tick uint64, kind interact.Kind, sights []Sight) {
	at := p.seen[tick]
	if at == nil {
		at = &seen{sights: map[uint32]sight{}}
		p.seen[tick] = at
	}
	at.synced |= kind
	shown := make(map[uint32]bool, len(sights))
	for _, s := range sights {
		at.sights[s.ID] = sight{side: s.Side, row: flight.Row{
			Tick: tick, Center: s.Center, Radius: s.Radius, Tags: s.Tags, Until: s.Until,
		}}
		shown[s.ID] = true
	}
	keep := p.content.Shots.RewindTicks()
	for t := range p.seen {
		if t+keep < tick {
			delete(p.seen, t)
		}
	}
	if newest, ok := p.newest[kind]; ok && tick < newest {
		return
	}
	p.newest[kind] = tick
	for _, s := range sights {
		p.ensure(s.ID, kind, s.Radius)
	}
	for id, t := range p.targets {
		if t.kind == kind && !shown[id] && id != p.state.Body {
			p.world.DestroyBody(t.body)
			delete(p.targets, id)
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
	clear(p.newest)
}

// ensure gives entity id a body of kind with one circle of radius at its origin, as the server's
// Bodies and Echoes have. Its filter meets only shots.
func (p *Predictor) ensure(id uint32, kind interact.Kind, radius float64) target {
	t, ok := p.targets[id]
	if ok && t.kind == kind && t.radius == radius {
		return t
	}
	if ok {
		p.world.DestroyBody(t.body)
	}
	def := box2d.DefaultBodyDef()
	def.Type = box2d.KinematicBody
	body := p.world.CreateBody(&def)
	p.world.SetBodyUserData(body, uint64(id))
	shape := box2d.DefaultShapeDef()
	shape.Filter = box2d.Filter{CategoryBits: uint64(kind), MaskBits: uint64(interact.Shot)}
	p.world.CreateCircleShape(body, &shape, &box2d.Circle{Radius: radius})
	t = target{body: body, kind: kind, radius: radius}
	p.targets[id] = t
	return t
}

// place puts each Body and Echo where view drew it (flight.AtView), and your Body where it is now with
// your tags st, as the server checks a claim. It returns whose each placed shape is, or nil when the
// view is too old or its two ticks' syncs aren't both here: then your shots meet no Body or Echo.
func (p *Predictor) place(view float64, st status.Status) flight.Targets {
	s := &p.state
	if !p.content.Shots.ValidView(view, s.Tick) {
		return nil
	}
	a := uint64(math.Floor(view))
	older, newer := p.seen[a], p.seen[a+1]
	both := interact.Body | interact.Echo
	if older == nil || newer == nil || older.synced != both || newer.synced != both {
		return nil
	}
	placed := map[uint32]flight.Target{}
	for id, t := range p.targets {
		n, ok := newer.sights[id]
		if !ok || id == s.Body {
			continue
		}
		o, hasOlder := older.sights[id]
		row, ok := flight.AtView(o.row, hasOlder, n.row, view)
		if !ok {
			continue
		}
		p.world.SetBodyTransform(p.ensure(id, t.kind, row.Radius).body, row.Center, box2d.RotIdentity)
		placed[id] = flight.Target{ID: id, Side: n.side, Tags: row.Tags}
	}
	p.world.SetBodyTransform(p.ensure(s.Body, interact.Body, s.Radius).body, s.Center, box2d.RotIdentity)
	placed[s.Body] = flight.Target{ID: s.Body, Side: interact.Side{Owner: s.Body, Team: s.Team}, Tags: st.Tags}
	return func(shape box2d.ShapeID) (flight.Target, bool) {
		t, ok := placed[uint32(p.world.BodyUserData(p.world.ShapeBody(shape)))]
		return t, ok
	}
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
// hook from where the move left your Body, its shots, then its effects on you from the tick after. Your
// shots meet the Bodies and Echoes where f's view drew them, and a shot that stops on one ends there.
// It returns your Body's new center, those stops as f's claims, and the shots that ended in the tick.
func (p *Predictor) Step(f Frame) (box2d.Vec2, []Claim, []Ended) { return p.step(f, true) }

// Replay runs the next tick for frame f again, after a Reset: the shots f claimed end, without flying,
// as the server ends them, and it makes no claims.
func (p *Predictor) Replay(f Frame) box2d.Vec2 {
	center, _, _ := p.step(f, false)
	return center
}

func (p *Predictor) step(f Frame, fresh bool) (box2d.Vec2, []Claim, []Ended) {
	s := &p.state
	s.Tick++
	st := status.Fold(slices.Values(s.Effects), s.Tick)
	aimX, aimY := motion.Unit(f.AimX, f.AimY)
	aim := hooks.Vec2{X: aimX, Y: aimY}
	var casts []string
	s.Cooldowns, casts = p.content.Abilities.Gate(s.Champion, s.Cooldowns, st.Tags, f.Buttons, aim, s.Tick)
	s.Center = p.move(f.MoveX, f.MoveY, st)
	var out hooks.Out
	var keys []Key // of out.Launches
	for _, id := range casts {
		if hook, ok := p.content.Abilities.Hook(id); ok && hook.OnCast != nil {
			n := len(out.Launches)
			hook.OnCast(hooks.Cast{
				Caster: s.Body, Owner: s.Body, Team: s.Team, // a player Body acts for itself
				Pos: hooks.Vec2(s.Center), Aim: aim, TickRate: timestep.Rate,
			}, &out)
			for i := range out.Launches[n:] {
				keys = append(keys, Key{Seq: f.Seq, Ability: id, Launch: uint32(i)})
			}
		}
	}
	var targets flight.Targets
	ended := f.Claims
	if fresh {
		targets, ended = p.place(f.ViewTick, st), nil
	}
	claims, ends := p.fly(out.Launches, keys, targets, ended)
	p.affect(out.Effects)
	return s.Center, claims, ends
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

// fly launches this tick's shots from your Body, then flies all your shots one tick. A shot ended
// names ends without flying. One that stops on a Body or Echo of targets ends there and is claimed;
// with no targets, shots pass Bodies and Echoes. It returns the claims and the shots that ended flying.
func (p *Predictor) fly(launches []hooks.Launch, keys []Key, targets flight.Targets, ended []Claim) ([]Claim, []Ended) {
	s := &p.state
	shots := p.content.Shots
	for i, l := range launches {
		if def, ok := shots.Shots[l.Shot]; ok {
			s.Shots = append(s.Shots, Shot{Type: l.Shot, Key: keys[i], Flight: flight.Launch(def, s.Center, box2d.Vec2(l.Dir))})
		}
	}
	side := interact.Side{Owner: s.Body, Team: s.Team}
	var claims []Claim
	var ends []Ended
	kept := s.Shots[:0]
	for _, shot := range s.Shots {
		def, ok := shots.Shots[shot.Type]
		if !ok || slices.ContainsFunc(ended, func(c Claim) bool { return c.Key == shot.Key }) {
			continue
		}
		next, end, over := shots.Fly(p.world, def, shot.Flight, side, targets, timestep.Seconds)
		if over {
			if end.Hit.Kind&(interact.Body|interact.Echo) != 0 {
				claims = append(claims, Claim{Key: shot.Key, Target: end.Hit.Target})
			}
			ends = append(ends, Ended{Shot: shot, End: end})
			continue
		}
		shot.Prev, shot.Flight = shot.Pos, next
		kept = append(kept, shot)
	}
	s.Shots = kept
	return claims, ends
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
