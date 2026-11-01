package status

import (
	"math"
	"slices"
	"testing"

	"echo_image/shards/game/internal/interact"
)

func TestSlowsTakeStrongestHastesAdd(t *testing.T) {
	var next uint32
	var l []Entry
	slow := func(bps int32, source uint32) Entry {
		return Entry{Kind: "slow", Tags: interact.CC, Stat: MoveSpeed, Value: bps, Source: source}
	}
	l, _ = Add(l, &next, slow(-4000, 7), PerSource, 10, 80)
	l, _ = Add(l, &next, slow(-4000, 8), PerSource, 10, 80)
	l, _ = Add(l, &next, slow(-2000, 9), PerSource, 10, 80)
	l, _ = Add(l, &next, Entry{Kind: "haste", Stat: MoveSpeed, Value: 500, Source: 7}, Merge, 10, 80)
	l, _ = Add(l, &next, Entry{Kind: "sprint", Stat: MoveSpeed, Value: 500, Source: 7}, Merge, 10, 80)
	if len(l) != 5 {
		t.Fatalf("len = %d, want 3 slows from 3 sources and 2 hastes", len(l))
	}
	if got := Fold(slices.Values(l), 10).Speed(5); math.Abs(got-5*1.1*0.6) > 1e-9 {
		t.Fatalf("speed = %v, want 5 x 1.1 x 0.6", got)
	}
}

func TestRefreshKeepsSerial(t *testing.T) {
	var next uint32
	var l []Entry
	l, _ = Add(l, &next, Entry{Kind: "slow", Source: 7}, PerSource, 10, 1)
	l, _ = Add(l, &next, Entry{Kind: "slow", Source: 7}, PerSource, 10, 5)
	l, _ = Add(l, &next, Entry{Kind: "stun", Source: 7}, Merge, 10, 1)
	l, _ = Add(l, &next, Entry{Kind: "stun", Source: 8}, Merge, 10, 5) // merge refreshes across sources
	want := []Entry{
		{Kind: "slow", Source: 7, Start: 10, End: 15, Serial: 1, Stacking: PerSource},
		{Kind: "stun", Source: 8, Start: 10, End: 15, Serial: 2},
	}
	if !slices.Equal(l, want) {
		t.Fatalf("got %+v, want %+v", l, want)
	}
}

// A refresh keeps what is strongest at every tick: a weaker, longer effect never erases a stronger one,
// and a stronger, shorter one isn't dropped. Each counts while it's the strongest of its refresh key.
func TestRefreshKeepsTheStrongest(t *testing.T) {
	slow := func(bps int32, source uint32) Entry {
		return Entry{Kind: "slow", Tags: interact.CC, Stat: MoveSpeed, Value: bps, Source: source}
	}
	haste := func(bps int32, source uint32) Entry {
		return Entry{Kind: "haste", Stat: MoveSpeed, Value: bps, Source: source}
	}
	type add struct {
		e     Entry
		rule  Stacking
		ticks uint64
	}
	for _, c := range []struct {
		name  string
		adds  []add
		speed map[uint64]float64 // at tick: speed from base 5
	}{
		{"strong then weak and longer", []add{{slow(-6000, 7), PerSource, 20}, {slow(-2000, 7), PerSource, 40}},
			map[uint64]float64{11: 2, 29: 2, 30: 4, 49: 4, 50: 5}},
		{"weak then strong and shorter", []add{{slow(-2000, 7), PerSource, 40}, {slow(-6000, 7), PerSource, 5}},
			map[uint64]float64{11: 2, 14: 2, 15: 4, 49: 4}},
		{"merged hastes from two sources", []add{{haste(5000, 7), Merge, 20}, {haste(1000, 8), Merge, 40}},
			map[uint64]float64{11: 7.5, 30: 5.5, 50: 5}},
		{"stronger and longer replaces", []add{{haste(1000, 7), Merge, 20}, {haste(5000, 8), Merge, 40}},
			map[uint64]float64{11: 7.5, 49: 7.5}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var next uint32
			var l []Entry
			for _, a := range c.adds {
				l, _ = Add(l, &next, a.e, a.rule, 10, a.ticks)
			}
			for tick, want := range c.speed {
				if got := Fold(slices.Values(l), tick).Speed(5); math.Abs(got-want) > 1e-9 {
					t.Errorf("speed at tick %d = %v, want %v (entries %+v)", tick, got, want, l)
				}
			}
		})
	}
}

// A short stun never cuts a long one short, whatever order they land in.
func TestRefreshKeepsWhicheverEndsLater(t *testing.T) {
	type add struct {
		source      uint32
		tick, ticks uint64
	}
	for _, adds := range [][]add{
		{{3, 10, 40}, {9, 10, 4}}, // one tick, long first
		{{9, 10, 4}, {3, 10, 40}}, // one tick, short first
		{{3, 10, 40}, {9, 20, 1}}, // a short one later
	} {
		var next uint32
		var l []Entry
		for _, a := range adds {
			l, _ = Add(l, &next, Entry{Kind: "stun", Source: a.source}, Merge, a.tick, a.ticks)
		}
		if len(l) != 1 || l[0].Source != 3 || l[0].End != 50 {
			t.Fatalf("adds %v: got %+v, want source 3's stun, ending at 50", adds, l)
		}
	}
}

// A stun ends your own dash but not a push from someone else, and stops input movement.
func TestCCEndsDashNotPush(t *testing.T) {
	var next uint32
	var l []Entry
	l, _ = Add(l, &next, Entry{Kind: "push", Tags: interact.CC, VelY: -15}, Instanced, 10, 6)
	l, _ = Add(l, &next, Entry{Kind: "dash", Tags: interact.EndsOnCC, VelX: 20}, Merge, 10, 6)
	if s := Fold(slices.Values(l), 11); !s.Forced || s.VelX != 20 || s.VelY != -15 {
		t.Fatalf("want the dash and the push summed: %+v", s)
	}
	l, _ = Add(l, &next, Entry{Kind: "stun", Tags: interact.Stunned | interact.CC}, Merge, 12, 10)
	s := Fold(slices.Values(l), 13)
	if s.VelX != 0 || s.VelY != -15 || s.Speed(5) != 0 {
		t.Fatalf("want the dash gone, the push left and no input speed: %+v", s)
	}
}

func TestUnstoppableBlocksCC(t *testing.T) {
	var next uint32
	l, _ := Add(nil, &next, Entry{Kind: "unstoppable", Tags: interact.Unstoppable}, Merge, 10, 20)
	l, ok := Add(l, &next, Entry{Kind: "push", Tags: interact.CC, VelX: -15}, Instanced, 14, 4)
	if ok || len(l) != 1 {
		t.Fatalf("unstoppable should block the push: %+v", l)
	}
	if _, ok := Add(l, &next, Entry{Kind: "push", Tags: interact.CC, VelX: -15}, Instanced, 30, 4); !ok {
		t.Fatal("a push should land once unstoppable is over")
	}
}

func TestEntryIsSweptWhenOver(t *testing.T) {
	var next uint32
	l, _ := Add(nil, &next, Entry{Kind: "phase", Pass: 1}, Merge, 10, 40)
	if Fold(slices.Values(l), 49).Pass != 1 || Fold(slices.Values(l), 50).Pass != 0 {
		t.Fatal("an entry applies from Start up to, not including, End")
	}
	if len(Sweep(slices.Clone(l), 49)) != 1 || len(Sweep(l, 50)) != 0 {
		t.Fatal("an entry is swept once it's over")
	}
}
