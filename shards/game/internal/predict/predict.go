// Package predict runs the client's own Body ahead of the server: its moves, its casts and their
// cooldowns, the effects they put on it, the shots it has in flight, and its Echo. It starts from the server's
// state after your last applied frame, and steps each frame the server hasn't applied yet as one tick
// of the server's own code, so with no surprises it lands on the server's bits. Your shots meet each
// target (a Body or Echo) where your screen drew it, as the server's FlightSystem judges them. What others
// do (effects they put on you, kills) comes with the next state. It imports no Cardinal:
// shards/game/predictor builds it for the browser.
package predict

import (
	"errors"
	"io/fs"
	"math"
	"reflect"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/ability/hooks"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/interact"
	"echo_image/shards/game/internal/physics/timestep"
	"echo_image/shards/game/internal/player/motion"
	"echo_image/shards/game/internal/projectile/flight"
)

// Content is the content the predictor runs: the server's own files, loaded as the shard loads them.
type Content struct {
	Abilities cast.Tuning
	Shots     flight.Tuning
	Effects   status.Tuning
}

// Load reads Content from fsys, the content embed.FS, with content.Load, as the shard does: each file
// decoded strictly, the hooks bound, then each checked.
func Load(fsys fs.FS) (Content, error) {
	var c Content
	var errs [3]error
	c.Abilities, errs[0] = content.Load[cast.Tuning](fsys)
	c.Shots, errs[1] = content.Load[flight.Tuning](fsys)
	c.Effects, errs[2] = content.Load[status.Tuning](fsys)
	return c, errors.Join(errs[:]...)
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
// come with the arena. It keeps the target syncs each frame's views are drawn from.
type Predictor struct {
	content Content
	world   *box2d.World
	state   State
	start   uint64                      // the state's tick at the last Reset
	moves   []box2d.Vec2                // your Body's center after each step since then
	seen    map[uint64]map[uint64]Sight // the last KeptRows ticks of target syncs: by tick, then serial
	newest  uint64                      // the newest tick seen
	stepped []stepped                   // each frame stepped since the last Reset, oldest first
}

// stepped is the state a frame's step left, without its EchoPath, and whether its shots flew without
// the target syncs of its view.
type stepped struct {
	seq   uint32
	state State
	blind bool
}

// New returns a Predictor that runs content. Reset it before stepping.
func New(content Content) *Predictor {
	def := box2d.DefaultWorldDef()
	return &Predictor{content: content, world: box2d.NewWorld(&def), seen: map[uint64]map[uint64]Sight{}}
}

// See keeps what the target sync of tick showed, so a frame can see each target across the views it
// is drawn over. The newest sync says which targets are still there: those that lived its whole tick,
// since the server removes one that died at the end of its tick, before any later frame's shots fly.
func (p *Predictor) See(tick uint64, sights []Sight) {
	at := make(map[uint64]Sight, len(sights))
	for _, s := range sights {
		s.Tick = tick
		at[s.Serial] = s
	}
	p.seen[tick], p.newest = at, max(p.newest, tick)
	keep := p.content.Shots.KeptRows()
	for t := range p.seen {
		if t+keep <= tick {
			delete(p.seen, t)
		}
	}
}

// Forget drops every sync it saw: after a reconnect or a server restart.
func (p *Predictor) Forget() {
	clear(p.seen)
	p.newest = 0
}

// sights is each target still there across the views a frame at view is drawn over (flight.Across), view
// held as the server holds it (flight.Tuning.View), for your shots to meet as FlightSystem judges them:
// your Body moving from prev to where it is now, all the tick with your tags st, and your Echo as you
// predict it (Echo). It returns no reach when a target sync of those views isn't here: then your shots
// meet no target.
func (p *Predictor) sights(view float64, prev box2d.Vec2, st status.Status) (*flight.Reach, flight.Sights) {
	s := &p.state
	view = p.content.Shots.View(view, s.Tick)
	for t := uint64(math.Floor(view)); t <= uint64(math.Ceil(view))+1; t++ {
		if _, ok := p.seen[t]; !ok {
			return nil, nil
		}
	}
	echo, echoPrev, hasEcho := p.Echo()
	reach, seen := flight.NewReach(), map[uint64]flight.Seen{}
	for serial, n := range p.seen[p.newest] {
		if n.Until < 1 {
			continue
		}
		v := flight.Seen{ID: n.ID, Serial: serial}
		var kept []flight.Row
		switch {
		case n.ID == s.Body:
			row := flight.Row{Kind: n.Kind, Side: interact.Side{Owner: s.Body, Team: s.Team}, Radius: s.Radius,
				Tags: st.Tags, Until: 1}
			v.Spans[0] = flight.Span{Row: row, End: 1}
			v.Move(prev, s.Center)
			kept = append(kept, flight.Row{Center: prev, Radius: s.Radius}, flight.Row{Center: s.Center, Radius: s.Radius})
		default:
			v.Spans = flight.Across(view, func(tick uint64) (flight.Row, bool) {
				sight, ok := p.seen[tick][serial]
				return sight.Row, ok
			})
			for _, sync := range p.seen {
				if sight, ok := sync[serial]; ok {
					kept = append(kept, sight.Row)
				}
			}
			if hasEcho && n.Kind == interact.Echo && n.Side.Owner == s.Body {
				v.Move(echoPrev, echo)
				kept = append(kept, flight.Row{Center: echoPrev, Radius: n.Radius}, flight.Row{Center: echo, Radius: n.Radius})
			}
		}
		if v.Shown() {
			seen[serial] = v
			reach.Add(serial, n.Row, kept...)
		}
	}
	return reach, func(key uint64) (flight.Seen, bool) {
		v, ok := seen[key]
		return v, ok
	}
}

// Reset starts over from s.
func (p *Predictor) Reset(s State) {
	s.Cooldowns, s.Effects, s.Shots = slices.Clone(s.Cooldowns), slices.Clone(s.Effects), slices.Clone(s.Shots)
	s.EchoPath = slices.Clone(s.EchoPath)
	p.state, p.start, p.moves, p.stepped = s, s.Tick, p.moves[:0], p.stepped[:0]
}

// Agrees reports whether s, the server's state after your frame seq, is the state stepping seq left,
// your Echo included up to the newest step, and every frame since flew your shots with its view's
// target syncs. Then those frames are right as they are, and the client skips its Reset and replay; the
// predictor takes the rest of s's Echo path, which it only guessed. It forgets the frames up to seq.
func (p *Predictor) Agrees(seq uint32, s State) bool {
	i := slices.IndexFunc(p.stepped, func(st stepped) bool { return st.seq == seq })
	if i < 0 {
		return false
	}
	was := p.stepped[i].state
	p.stepped = p.stepped[i+1:]
	if slices.ContainsFunc(p.stepped, func(st stepped) bool { return st.blind }) || !same(was, s) ||
		len(s.EchoPath) > 0 != (len(p.state.EchoPath) > 0) {
		return false
	}
	k := int(was.Tick - p.start)
	for i, at := range s.EchoPath {
		if s.Tick+uint64(i) <= p.state.Tick && at != p.echoAt(k+i) {
			return false
		}
	}
	p.start, p.moves, p.state.EchoPath = s.Tick, p.moves[k:], slices.Clone(s.EchoPath)
	return true
}

// same reports whether a and b are the same state but for EchoPath, an empty list being the same as
// none.
func same(a, b State) bool {
	for _, s := range []*State{&a, &b} {
		s.EchoPath = nil
		if len(s.Cooldowns) == 0 {
			s.Cooldowns = nil
		}
		if len(s.Effects) == 0 {
			s.Effects = nil
		}
		if len(s.Shots) == 0 {
			s.Shots = nil
		}
	}
	return reflect.DeepEqual(a, b)
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
	prev := s.Center
	s.Center = p.move(f.MoveX, f.MoveY, st)
	p.moves = append(p.moves, s.Center)
	asked := asks{body: s.Body}
	for _, id := range casts {
		p.content.Abilities.Run(id, hooks.Cast{
			Caster: s.Body, Owner: s.Body, Team: s.Team, // a player Body acts for itself
			Pos: hooks.Vec2(s.Center), Aim: aim,
		}, &asked)
	}
	reach, sights := p.sights(f.ViewTick, prev, st)
	blind := reach == nil && (len(s.Shots) > 0 || len(asked.launches) > 0)
	ends := p.fly(asked.launches, f.Seq, reach, sights)
	p.affect(asked.effects)
	was := p.State()
	was.EchoPath = nil
	p.stepped = append(p.stepped, stepped{seq: f.Seq, state: was, blind: blind})
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
// on a Body or Echo sights shows ends there; with no reach, shots pass Bodies and Echoes. It returns the
// shots that ended.
func (p *Predictor) fly(launches []cast.Launch, seq uint32, reach *flight.Reach, sights flight.Sights) []Ended {
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
		next, end, over := shots.Fly(p.world, def, shot.Flight, side, reach, sights, timestep.Seconds)
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
