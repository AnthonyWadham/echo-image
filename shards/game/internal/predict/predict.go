// Package predict runs the client's own Body ahead of the server: its moves, its casts and their
// cooldowns, the effects they put on it, the shots it has in flight, and its Echo. It starts from the server's
// state after your last applied frame, and steps each frame the server hasn't applied yet as one tick
// of the server's own code, so with no surprises it lands on the server's bits. Your shots meet each
// recorded thing (a Body or Echo) where your screen drew it, as the server's FlightSystem judges them. What others
// do (effects they put on you, kills) comes with the next state. It imports no Cardinal:
// shards/game/predictor builds it for the browser.
package predict

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"reflect"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"

	"echo_image/pkg/level"
	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability/cast"
	"echo_image/shards/game/internal/ability/hooks"
	"echo_image/shards/game/internal/arena"
	"echo_image/shards/game/internal/effect/status"
	"echo_image/shards/game/internal/history/past"
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
	History   past.Tuning
	Maps      map[string]*box2d.World // each map's walls, by ID: queried, never stepped
}

// Load reads Content from fsys, the content embed.FS, with content.Load, as the shard does: each file
// decoded strictly, the hooks bound, then each checked. Each map is built as the shard builds it.
func Load(fsys fs.FS) (Content, error) {
	var c Content
	var errs [5]error
	c.Abilities, errs[0] = content.Load[cast.Tuning](fsys)
	c.Shots, errs[1] = content.Load[flight.Tuning](fsys)
	c.Effects, errs[2] = content.Load[status.Tuning](fsys)
	c.History, errs[3] = content.Load[past.Tuning](fsys)
	c.Maps, errs[4] = loadMaps(fsys)
	return c, errors.Join(errs[:]...)
}

// loadMaps builds each map content/arena lists into a Box2D world of its own, from the files the shard's
// map plugin builds.
func loadMaps(fsys fs.FS) (map[string]*box2d.World, error) {
	t, err := content.Load[arena.ArenaTuning](fsys)
	if err != nil {
		return nil, err
	}
	files, err := t.Files(fsys)
	if err != nil {
		return nil, err
	}
	schema := arena.Schema()
	worlds := make(map[string]*box2d.World, len(files))
	for _, id := range t.Maps {
		m, err := level.Decode(files[id], schema)
		if err != nil {
			return nil, fmt.Errorf("map %s: %w", id, err)
		}
		def := box2d.DefaultWorldDef()
		worlds[id] = box2d.NewWorld(&def)
		if _, err := level.Build(worlds[id], m, schema); err != nil {
			return nil, fmt.Errorf("map %s: %w", id, err)
		}
	}
	return worlds, nil
}

// State is the server's state of your Body and what it started, after Tick, as body_sync sends it.
type State struct {
	Tick        uint64
	Map         string // the live map's ID: its walls
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
	EchoSerial  uint64 // its Echo's serial: 0 with no Echo
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

// Sight is a thing as a target sync showed it: its entity and serial, and its row. The predictor keeps
// what it saw by Serial, since a newer entity can take ID.
type Sight struct {
	ID     uint32
	Serial uint64
	past.Row
}

// Predictor steps your Body one tick per frame from the last State, through the walls of the map it
// names. It keeps each thing's rows from the target syncs, as the server keeps its History, for the views
// each frame is drawn over.
type Predictor struct {
	content  Content
	world    *box2d.World // the state's map
	state    State
	start    uint64       // the state's tick at the last Reset
	moves    []box2d.Vec2 // your Body's center after each step since then
	recorded []recorded   // each thing the kept syncs show, by serial
	synced   []uint64     // the kept syncs' ticks: a ring, tick t at t % KeptRows, math.MaxUint64 for none
	newest   uint64       // the newest tick seen
	scene    *past.Scene  // what a step's judges meet
	kept     []past.Row   // scratch: one thing's kept rows
	stepped  []stepped    // each frame stepped since the last Reset, oldest first
}

// recorded is one thing's rows from the kept syncs: a ring, tick t's at t % KeptRows. A slot of another
// tick holds none of it.
type recorded struct {
	id     uint32
	serial uint64
	last   uint64 // the newest tick a sync showed it
	rows   []past.Row
}

// at is t's row of tick, if a kept sync showed it.
func (t *recorded) at(tick uint64) (past.Row, bool) {
	if r := t.rows[tick%uint64(len(t.rows))]; r.Tick == tick {
		return r, true
	}
	return past.Row{}, false
}

// appendRows appends t's rows of the ticks before before that its ring spans, oldest first.
func (t *recorded) appendRows(dst []past.Row, before uint64) []past.Row {
	for tick := before - min(before, uint64(len(t.rows))); tick < before; tick++ {
		if r, ok := t.at(tick); ok {
			dst = append(dst, r)
		}
	}
	return dst
}

// stepped is the state a frame's step left, without its EchoPath, and whether a judge looked without
// the target syncs of its view.
type stepped struct {
	seq   uint32
	state State
	blind bool
}

// New returns a Predictor that runs content. Reset it before stepping.
func New(content Content) *Predictor {
	p := &Predictor{content: content, scene: past.NewScene(content.History.Window),
		synced: make([]uint64, content.History.Window.KeptRows())}
	p.Forget()
	return p
}

// See keeps what the target sync of tick showed, so a frame can see each thing across the views it is
// drawn over: each one's row by serial, as many ticks as the server keeps rows (Window.KeptRows). A sync
// older than those is dropped. The newest sync says which things are still there: those that lived its
// whole tick, since the server removes one that died at the end of its tick, before any later frame's
// shots fly. O(T log T) for T things, with no allocations but a new thing's ring.
func (p *Predictor) See(tick uint64, sights []Sight) {
	keep := uint64(len(p.synced))
	if tick+keep <= p.newest {
		return
	}
	p.newest, p.synced[tick%keep] = max(p.newest, tick), tick
	for _, s := range sights {
		s.Tick = tick
		i, ok := slices.BinarySearchFunc(p.recorded, s.Serial, func(t recorded, serial uint64) int {
			return cmp.Compare(t.serial, serial)
		})
		if !ok { // sized at its first row: every other slot holds it, at a tick not its own
			p.recorded = slices.Insert(p.recorded, i, recorded{id: s.ID, serial: s.Serial,
				rows: slices.Repeat([]past.Row{s.Row}, int(keep))})
		}
		t := &p.recorded[i]
		t.rows[tick%keep], t.last = s.Row, max(t.last, tick)
	}
	p.recorded = slices.DeleteFunc(p.recorded, func(t recorded) bool { return t.last+keep <= p.newest })
}

// Forget drops every sync it saw: after a reconnect or a server restart.
func (p *Predictor) Forget() {
	p.recorded, p.newest = p.recorded[:0], 0
	for i := range p.synced {
		p.synced[i] = math.MaxUint64
	}
}

// sceneAt is the actor your frame at view acts as, its view held as the server holds it
// (past.Window.Clamp), and the scene its judges meet, as the server's do: each thing still there, by
// its kept rows, your Body moving from prev to where it is now and your Echo as you predict it (Echo).
// No scene when a target sync of the view's ticks isn't here: then they meet no thing.
func (p *Predictor) sceneAt(view float64, prev box2d.Vec2) (past.Actor, *past.Scene) {
	s := &p.state
	a := past.Actor{Side: interact.Side{Owner: s.Body, Team: s.Team},
		View: p.content.History.Window.Clamp(view, s.Tick), Predicted: true}
	keep := uint64(len(p.synced))
	for t := uint64(math.Floor(a.View)); t <= uint64(math.Ceil(a.View))+1; t++ {
		if p.synced[t%keep] != t {
			return a, nil
		}
	}
	echo, echoPrev, hasEcho := p.Echo()
	p.scene.Reset(s.Tick)
	for i := range p.recorded {
		t := &p.recorded[i]
		now, ok := t.at(p.newest)
		if !ok || now.Until < 1 {
			continue
		}
		th := past.Thing{ID: t.id, Serial: t.serial, Now: now, From: now.Center, To: now.Center}
		switch {
		case t.id == s.Body:
			th.Predicted, th.From, th.To = true, prev, s.Center
		case hasEcho && t.serial == s.EchoSerial:
			th.Predicted, th.From, th.To = true, echoPrev, echo
		}
		p.kept = t.appendRows(p.kept[:0], p.newest)
		p.scene.Add(th, p.kept)
	}
	p.scene.Build()
	return a, p.scene
}

// Reset starts over from s. Its map must be one of the content's, as a shard of the same build names.
func (p *Predictor) Reset(s State) {
	world, ok := p.content.Maps[s.Map]
	if !ok {
		panic(fmt.Sprintf("predict: no map %q in this content", s.Map))
	}
	p.world = world
	s.Cooldowns, s.Effects, s.Shots = slices.Clone(s.Cooldowns), slices.Clone(s.Effects), slices.Clone(s.Shots)
	s.EchoPath = slices.Clone(s.EchoPath)
	p.state, p.start, p.moves, p.stepped = s, s.Tick, p.moves[:0], p.stepped[:0]
}

// Agrees reports whether s, the server's state after your frame seq, is the state stepping seq left,
// your Echo included up to the newest step, and no frame since is blind. Then those frames are right as
// they are, and the client skips its Reset and replay; the predictor takes the rest of s's Echo path,
// which it only guessed. It forgets the frames up to seq.
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
	// Each judge gets the scene by look, built when the first one looks: one that looks without the
	// view's syncs makes the step blind.
	var a past.Actor
	var scene *past.Scene
	looked := false
	look := func() (past.Actor, *past.Scene) {
		if !looked {
			a, scene = p.sceneAt(f.ViewTick, prev)
			looked = true
		}
		return a, scene
	}
	ends := p.fly(asked.launches, f.Seq, look)
	p.affect(asked.effects)
	was := p.State()
	was.EchoPath = nil
	p.stepped = append(p.stepped, stepped{seq: f.Seq, state: was, blind: looked && scene == nil})
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

// fly launches the shots frame seq's casts asked for, then flies all your shots one tick in the scene
// look gives. One that stops on a thing there ends there; with no scene, shots pass every thing. It
// returns the shots that ended.
func (p *Predictor) fly(launches []cast.Launch, seq uint32, look func() (past.Actor, *past.Scene)) []Ended {
	s := &p.state
	shots := p.content.Shots
	for _, l := range launches {
		if def, ok := shots.Shots[l.Shot]; ok {
			key := Key{Seq: seq, Ability: l.Ability, Launch: l.Launch}
			s.Shots = append(s.Shots, Shot{Type: l.Shot, Key: key,
				Flight: flight.Launch(def, box2d.Vec2(l.From), box2d.Vec2(l.Dir))})
		}
	}
	var ends []Ended
	kept := s.Shots[:0]
	for _, shot := range s.Shots {
		def, ok := shots.Shots[shot.Type]
		if !ok {
			continue
		}
		a, scene := look()
		next, end, over := shots.Fly(p.world, def, shot.Flight, a, scene, timestep.Seconds)
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
