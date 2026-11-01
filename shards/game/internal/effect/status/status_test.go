package status

import (
	"encoding/json"
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
	l = Add(l, &next, slow(-4000, 7), PerSource, 10, 80)
	l = Add(l, &next, slow(-4000, 8), PerSource, 10, 80)
	l = Add(l, &next, slow(-2000, 9), PerSource, 10, 80)
	l = Add(l, &next, Entry{Kind: "haste", Stat: MoveSpeed, Value: 500, Source: 7}, Merge, 10, 80)
	l = Add(l, &next, Entry{Kind: "sprint", Stat: MoveSpeed, Value: 500, Source: 7}, Merge, 10, 80)
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
	l = Add(l, &next, Entry{Kind: "slow", Source: 7}, PerSource, 10, 1)
	l = Add(l, &next, Entry{Kind: "slow", Source: 7}, PerSource, 10, 5)
	l = Add(l, &next, Entry{Kind: "stun", Source: 7}, Merge, 10, 1)
	l = Add(l, &next, Entry{Kind: "stun", Source: 8}, Merge, 10, 5) // merge refreshes across sources
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
				l = Add(l, &next, a.e, a.rule, 10, a.ticks)
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
			l = Add(l, &next, Entry{Kind: "stun", Source: a.source}, Merge, a.tick, a.ticks)
		}
		if len(l) != 1 || l[0].Source != 3 || l[0].End != 50 {
			t.Fatalf("adds %v: got %+v, want source 3's stun, ending at 50", adds, l)
		}
	}
}

// kinds is test content: what each kind ends as it lands, and the tags that block it.
func kinds(t *testing.T) Tuning {
	t.Helper()
	var tu Tuning
	if err := json.Unmarshal([]byte(`{"kinds": {
		"push": {"tags": ["cc"], "blocked_by": ["unstoppable"], "stacking": "instanced"},
		"dash": {"tags": ["ends_on_cc"], "stacking": "merge"},
		"stun": {"tags": ["stunned", "cc"], "ends": ["ends_on_cc"], "blocked_by": ["unstoppable"], "stacking": "merge"},
		"slow": {"tags": ["cc"], "stat": "move_speed", "blocked_by": ["unstoppable"], "stacking": "per_source"},
		"unstoppable": {"tags": ["unstoppable"], "stacking": "merge"}}}`), &tu); err != nil {
		t.Fatal(err)
	}
	return tu
}

// A stun ends your own dash, as its kind says, but not a push from someone else, and stops input
// movement. A slow is cc too, but ends nothing.
func TestKindEndsWhatItSays(t *testing.T) {
	tu, next := kinds(t), uint32(0)
	var l []Entry
	l, _ = tu.Apply(l, &next, Intent{Kind: "push", Ticks: 6, VelY: -15}, 10)
	l, _ = tu.Apply(l, &next, Intent{Kind: "dash", Ticks: 6, VelX: 20}, 10)
	l, _ = tu.Apply(l, &next, Intent{Kind: "slow", Ticks: 6, Value: -1000, Source: 4}, 11)
	if s := Fold(slices.Values(l), 11); !s.Forced || s.VelX != 20 || s.VelY != -15 {
		t.Fatalf("want the dash and the push summed, a slow ending neither: %+v", s)
	}
	l, _ = tu.Apply(l, &next, Intent{Kind: "stun", Ticks: 10}, 12)
	s := Fold(slices.Values(l), 13)
	if s.VelX != 0 || s.VelY != -15 || s.Speed(5) != 0 {
		t.Fatalf("want the dash gone, the push left and no input speed: %+v", s)
	}
}

func TestBlockedByTagsNeverLand(t *testing.T) {
	tu, next := kinds(t), uint32(0)
	l, _ := tu.Apply(nil, &next, Intent{Kind: "unstoppable", Ticks: 20}, 10)
	for _, kind := range []string{"push", "stun", "slow"} {
		if got, ok := tu.Apply(l, &next, Intent{Kind: kind, Ticks: 4, VelX: -15, Value: -1000}, 14); !ok || len(got) != 1 {
			t.Fatalf("unstoppable should block the %s: %+v", kind, got)
		}
	}
	if got, _ := tu.Apply(l, &next, Intent{Kind: "push", Ticks: 4, VelX: -15}, 30); len(got) != 2 {
		t.Fatal("a push should land once unstoppable is over")
	}
}

// Intents for one target land in Compare's order: a kind something can block lands last, so an
// unstoppable from the same tick blocks a stun, whichever was emitted first.
func TestSameTickUnstoppableBlocksStun(t *testing.T) {
	tu, next := kinds(t), uint32(0)
	intents := []Intent{{Target: 1, Kind: "stun", Ticks: 10, Source: 2}, {Target: 1, Kind: "unstoppable", Ticks: 10, Source: 9}}
	slices.SortFunc(intents, tu.Compare)
	var l []Entry
	for _, in := range intents {
		l, _ = tu.Apply(l, &next, in, 10)
	}
	if s := Fold(slices.Values(l), 11); s.Tags&interact.Stunned != 0 || s.Tags&interact.Unstoppable == 0 {
		t.Fatalf("want unstoppable and no stun: %+v", l)
	}
}

func TestEntryIsSweptWhenOver(t *testing.T) {
	var next uint32
	l := Add(nil, &next, Entry{Kind: "phase", Pass: 1}, Merge, 10, 40)
	if Fold(slices.Values(l), 49).Pass != 1 || Fold(slices.Values(l), 50).Pass != 0 {
		t.Fatal("an entry applies from Start up to, not including, End")
	}
	if len(Sweep(slices.Clone(l), 49)) != 1 || len(Sweep(l, 50)) != 0 {
		t.Fatal("an entry is swept once it's over")
	}
}

// Other screens see each active kind once, until its last entry ends.
func TestShownIsEachKindUntilItsLastEnd(t *testing.T) {
	var next uint32
	var l []Entry
	l = Add(l, &next, Entry{Kind: "slow", Value: -4000, Source: 7}, PerSource, 10, 8)
	l = Add(l, &next, Entry{Kind: "slow", Value: -2000, Source: 8}, PerSource, 10, 5)
	l = Add(l, &next, Entry{Kind: "dash"}, Merge, 10, 3)
	l = Add(l, &next, Entry{Kind: "stun"}, Merge, 20, 3) // not yet
	want := []Shown{{Kind: "dash", End: 13}, {Kind: "slow", End: 18}}
	if got := ShownAt(slices.Values(l), 12); !slices.Equal(got, want) {
		t.Fatalf("shown %+v, want %+v", got, want)
	}
}
