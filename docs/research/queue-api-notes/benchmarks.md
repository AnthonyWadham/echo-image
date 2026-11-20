# Queue research: benchmarks

Run 2026-11-19 during the [queue API research](../queue-api.md), where they are source [4]. Machine:
11th Gen Intel i9-11900K, linux/amd64 (from the output headers). `bench/immutable` was a copy of World
Engine main's `pkg/immutable` (after #1177), so the benchmarks ran on the new `Slice`. That copy isn't
kept here.

## bench_test.go

```go
package qbench

import (
	"cmp"
	"math/rand/v2"
	"slices"
	"sort"
	"testing"

	"qbench/immutable"
)

// Header is the request header from the brief: due tick, source, cause, depth, flags, plus a seq tie-breaker.
type Header struct {
	Due    uint64
	Source uint64
	Cause  uint32
	Seq    uint32
	Depth  uint8
	Flags  uint8
}

type Req struct {
	H      Header
	Target uint64
	Amount int64
}

func (r Req) Hdr() Header { return r.H }

func cmpReq(a, b Req) int {
	if c := cmp.Compare(a.H.Due, b.H.Due); c != 0 {
		return c
	}
	if c := cmp.Compare(a.H.Source, b.H.Source); c != 0 {
		return c
	}
	return cmp.Compare(a.H.Seq, b.H.Seq)
}

func gen(n int) []Req {
	r := rand.New(rand.NewPCG(1, 2))
	out := make([]Req, n)
	for i := range out {
		out[i] = Req{H: Header{Due: uint64(r.IntN(4)), Source: uint64(r.IntN(500)), Seq: uint32(i)}, Target: uint64(r.IntN(500)), Amount: 1}
	}
	return out
}

var sizes = []struct {
	name string
	n    int
}{{"n10", 10}, {"n100", 100}, {"n1000", 1000}}

func BenchmarkSort(b *testing.B) {
	for _, sz := range sizes {
		src := gen(sz.n)
		buf := make([]Req, sz.n)
		b.Run("sort.Slice/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				copy(buf, src)
				sort.Slice(buf, func(i, j int) bool { return cmpReq(buf[i], buf[j]) < 0 })
			}
		})
		b.Run("sort.SliceStable/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				copy(buf, src)
				sort.SliceStable(buf, func(i, j int) bool { return cmpReq(buf[i], buf[j]) < 0 })
			}
		})
		b.Run("slices.SortFunc/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				copy(buf, src)
				slices.SortFunc(buf, cmpReq)
			}
		})
		b.Run("slices.SortStableFunc/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				copy(buf, src)
				slices.SortStableFunc(buf, cmpReq)
			}
		})
		b.Run("immutable.SortStableFunc/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s := immutable.SliceOf(src...)
				s.SortStableFunc(cmpReq)
			}
		})
	}
}

// Enqueue n requests one at a time, then drain and reset: one tick of a queue.
func BenchmarkEnqueueDrain(b *testing.B) {
	for _, sz := range sizes {
		src := gen(sz.n)
		b.Run("plain-reused/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			var q []Req
			var sum int64
			for b.Loop() {
				for _, r := range src {
					q = append(q, r)
				}
				for _, r := range q {
					sum += r.Amount
				}
				q = q[:0]
			}
			_ = sum
		})
		b.Run("immutable-Append/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			var sum int64
			for b.Loop() {
				var q immutable.Slice[Req]
				for _, r := range src {
					q.Append(r)
				}
				for r := range q.Values() {
					sum += r.Amount
				}
				q.Reset()
			}
			_ = sum
		})
		b.Run("immutable-Insert-Len/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			var sum int64
			for b.Loop() {
				var q immutable.Slice[Req]
				for _, r := range src {
					q.Insert(q.Len(), r)
				}
				for r := range q.Values() {
					sum += r.Amount
				}
			}
			_ = sum
		})
	}
}

// Header read via a constraint method vs a direct field, inside a generic drain.
type HasHdr interface{ Hdr() Header }

//go:noinline
func sumDueMethod[R HasHdr](q []R) (s uint64) {
	for _, r := range q {
		s += r.Hdr().Due
	}
	return
}

//go:noinline
func sumDueField(q []Req) (s uint64) {
	for _, r := range q {
		s += r.H.Due
	}
	return
}

type Queued[P any] struct {
	H Header
	P P
}

//go:noinline
func sumDueWrapped[P any](q []Queued[P]) (s uint64) {
	for _, r := range q {
		s += r.H.Due
	}
	return
}

func BenchmarkHeaderAccess(b *testing.B) {
	src := gen(1000)
	w := make([]Queued[[2]uint64], 1000)
	for i := range w {
		w[i].H = src[i].H
	}
	var s uint64
	b.Run("generic-method", func(b *testing.B) {
		for b.Loop() {
			s += sumDueMethod(src)
		}
	})
	b.Run("concrete-field", func(b *testing.B) {
		for b.Loop() {
			s += sumDueField(src)
		}
	})
	b.Run("generic-wrapper-field", func(b *testing.B) {
		for b.Loop() {
			s += sumDueWrapped(w)
		}
	})
	_ = s
}

// Copy cost of a component value on Get/Set: small header struct, fixed array of 16 requests, immutable.Slice.
type CompSlice struct{ Q immutable.Slice[Req] }
type CompArr16 struct {
	N int
	Q [16]Req
}
type CompArr64 struct {
	N int
	Q [64]Req
}

var sinkS CompSlice
var sinkA CompArr16
var sinkB CompArr64

func BenchmarkComponentCopy(b *testing.B) {
	cs := CompSlice{Q: immutable.SliceOf(gen(1000)...)}
	var ca CompArr16
	var cb CompArr64
	b.Run("immutable.Slice-1000", func(b *testing.B) {
		for b.Loop() {
			sinkS = cs
		}
	})
	b.Run("array16", func(b *testing.B) {
		for b.Loop() {
			sinkA = ca
		}
	})
	b.Run("array64", func(b *testing.B) {
		for b.Loop() {
			sinkB = cb
		}
	})
}
```

## batch_test.go

```go
package qbench

import (
	"testing"

	"qbench/immutable"
)

// Senders buffer in a plain local slice, then one Append per sender per tick.
func BenchmarkEnqueueBatch(b *testing.B) {
	for _, sz := range sizes {
		src := gen(sz.n)
		b.Run("immutable-one-Append/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			var sum int64
			for b.Loop() {
				var q immutable.Slice[Req]
				q.Append(src...)
				for r := range q.Values() {
					sum += r.Amount
				}
			}
			_ = sum
		})
		b.Run("immutable-10-senders/"+sz.name, func(b *testing.B) {
			b.ReportAllocs()
			var sum int64
			step := max(sz.n/10, 1)
			for b.Loop() {
				var q immutable.Slice[Req]
				for i := 0; i < sz.n; i += step {
					q.Append(src[i:min(i+step, sz.n)]...)
				}
				for r := range q.Values() {
					sum += r.Amount
				}
			}
			_ = sum
		})
	}
}
```

## go.mod

```text
module qbench

go 1.27.4

require (
	github.com/goccy/go-json v0.11.2
	github.com/rotisserie/eris v0.5.4
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
)
```

## Output: bench-out.txt

```text
goos: linux
goarch: amd64
pkg: qbench
cpu: 11th Gen Intel(R) Core(TM) i9-11900K @ 3.50GHz
BenchmarkSort/sort.Slice/n10-16         	  594643	       404.0 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.Slice/n10-16         	  597477	       404.2 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.Slice/n10-16         	  677188	       411.0 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.SliceStable/n10-16   	  651552	       406.7 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.SliceStable/n10-16   	  667249	       391.7 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.SliceStable/n10-16   	  471696	       426.0 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/slices.SortFunc/n10-16    	 1447556	       164.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortFunc/n10-16    	 1459831	       162.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortFunc/n10-16    	 1509477	       157.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortStableFunc/n10-16         	 1619271	       148.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortStableFunc/n10-16         	 1618503	       148.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortStableFunc/n10-16         	 1616316	       148.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/immutable.SortStableFunc/n10-16      	  464352	       439.7 ns/op	     960 B/op	       2 allocs/op
BenchmarkSort/immutable.SortStableFunc/n10-16      	  650190	       435.4 ns/op	     960 B/op	       2 allocs/op
BenchmarkSort/immutable.SortStableFunc/n10-16      	  614169	       420.3 ns/op	     960 B/op	       2 allocs/op
BenchmarkSort/sort.Slice/n100-16                   	   34364	      6961 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.Slice/n100-16                   	   33150	      7670 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.Slice/n100-16                   	   33106	      7072 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.SliceStable/n100-16             	   13683	     17529 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.SliceStable/n100-16             	   13374	     18078 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.SliceStable/n100-16             	   12753	     18446 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/slices.SortFunc/n100-16              	   55346	      4323 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortFunc/n100-16              	   54848	      4468 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortFunc/n100-16              	   55351	      4384 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortStableFunc/n100-16        	   30822	      7814 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortStableFunc/n100-16        	   31820	      7430 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortStableFunc/n100-16        	   32602	      7558 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/immutable.SortStableFunc/n100-16     	   32373	      7328 ns/op	   11568 B/op	      13 allocs/op
BenchmarkSort/immutable.SortStableFunc/n100-16     	   32725	      7576 ns/op	   11568 B/op	      13 allocs/op
BenchmarkSort/immutable.SortStableFunc/n100-16     	   33224	      7425 ns/op	   11568 B/op	      13 allocs/op
BenchmarkSort/sort.Slice/n1000-16                  	    1639	    140314 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.Slice/n1000-16                  	    1634	    144383 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.Slice/n1000-16                  	    1633	    143640 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.SliceStable/n1000-16            	     618	    390256 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.SliceStable/n1000-16            	     634	    379550 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/sort.SliceStable/n1000-16            	     633	    379668 ns/op	     120 B/op	       3 allocs/op
BenchmarkSort/slices.SortFunc/n1000-16             	    2846	     80880 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortFunc/n1000-16             	    2988	     82086 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortFunc/n1000-16             	    3094	     81303 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortStableFunc/n1000-16       	    1438	    164751 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortStableFunc/n1000-16       	    1394	    179916 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/slices.SortStableFunc/n1000-16       	    1446	    170072 ns/op	       0 B/op	       0 allocs/op
BenchmarkSort/immutable.SortStableFunc/n1000-16    	    1630	    131944 ns/op	  110640 B/op	      71 allocs/op
BenchmarkSort/immutable.SortStableFunc/n1000-16    	    1626	    136202 ns/op	  110640 B/op	      71 allocs/op
BenchmarkSort/immutable.SortStableFunc/n1000-16    	    1815	    126516 ns/op	  110640 B/op	      71 allocs/op
BenchmarkEnqueueDrain/plain-reused/n10-16          	10245735	        23.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkEnqueueDrain/plain-reused/n10-16          	10497129	        23.39 ns/op	       0 B/op	       0 allocs/op
BenchmarkEnqueueDrain/plain-reused/n10-16          	10581116	        23.13 ns/op	       0 B/op	       0 allocs/op
BenchmarkEnqueueDrain/immutable-Append/n10-16      	  277896	       867.8 ns/op	    2672 B/op	      10 allocs/op
BenchmarkEnqueueDrain/immutable-Append/n10-16      	  276283	       933.1 ns/op	    2672 B/op	      10 allocs/op
BenchmarkEnqueueDrain/immutable-Append/n10-16      	  256118	       886.3 ns/op	    2672 B/op	      10 allocs/op
BenchmarkEnqueueDrain/immutable-Insert-Len/n10-16  	  276196	       963.1 ns/op	    2672 B/op	      10 allocs/op
BenchmarkEnqueueDrain/immutable-Insert-Len/n10-16  	  279999	       920.8 ns/op	    2672 B/op	      10 allocs/op
BenchmarkEnqueueDrain/immutable-Insert-Len/n10-16  	  227269	       925.5 ns/op	    2672 B/op	      10 allocs/op
BenchmarkEnqueueDrain/plain-reused/n100-16         	 1000000	       242.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkEnqueueDrain/plain-reused/n100-16         	  962805	       250.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkEnqueueDrain/plain-reused/n100-16         	  944714	       272.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkEnqueueDrain/immutable-Append/n100-16     	    9645	     24875 ns/op	   80512 B/op	     105 allocs/op
BenchmarkEnqueueDrain/immutable-Append/n100-16     	   10000	     23339 ns/op	   80512 B/op	     105 allocs/op
BenchmarkEnqueueDrain/immutable-Append/n100-16     	   10000	     25839 ns/op	   80512 B/op	     105 allocs/op
BenchmarkEnqueueDrain/immutable-Insert-Len/n100-16 	    9152	     24677 ns/op	   80512 B/op	     105 allocs/op
BenchmarkEnqueueDrain/immutable-Insert-Len/n100-16 	    9464	     24895 ns/op	   80512 B/op	     105 allocs/op
BenchmarkEnqueueDrain/immutable-Insert-Len/n100-16 	   10183	     23591 ns/op	   80512 B/op	     105 allocs/op
BenchmarkEnqueueDrain/plain-reused/n1000-16        	   89731	      2686 ns/op	       1 B/op	       0 allocs/op
BenchmarkEnqueueDrain/plain-reused/n1000-16        	   86150	      2781 ns/op	       1 B/op	       0 allocs/op
BenchmarkEnqueueDrain/plain-reused/n1000-16        	   90778	      2746 ns/op	       1 B/op	       0 allocs/op
BenchmarkEnqueueDrain/immutable-Append/n1000-16    	     993	    246174 ns/op	  831737 B/op	    1061 allocs/op
BenchmarkEnqueueDrain/immutable-Append/n1000-16    	    1090	    250562 ns/op	  831736 B/op	    1061 allocs/op
BenchmarkEnqueueDrain/immutable-Append/n1000-16    	     902	    269950 ns/op	  831734 B/op	    1061 allocs/op
BenchmarkEnqueueDrain/immutable-Insert-Len/n1000-16         	    1000	    257528 ns/op	  831736 B/op	    1061 allocs/op
BenchmarkEnqueueDrain/immutable-Insert-Len/n1000-16         	     883	    250176 ns/op	  831736 B/op	    1061 allocs/op
BenchmarkEnqueueDrain/immutable-Insert-Len/n1000-16         	     961	    253230 ns/op	  831737 B/op	    1061 allocs/op
BenchmarkHeaderAccess/generic-method-16                     	   34369	      6944 ns/op
BenchmarkHeaderAccess/generic-method-16                     	   35403	      6828 ns/op
BenchmarkHeaderAccess/generic-method-16                     	   36727	      6521 ns/op
BenchmarkHeaderAccess/concrete-field-16                     	  617000	       381.1 ns/op
BenchmarkHeaderAccess/concrete-field-16                     	  635168	       379.2 ns/op
BenchmarkHeaderAccess/concrete-field-16                     	  629256	       391.5 ns/op
BenchmarkHeaderAccess/generic-wrapper-field-16              	  659200	       375.4 ns/op
BenchmarkHeaderAccess/generic-wrapper-field-16              	  672064	       382.8 ns/op
BenchmarkHeaderAccess/generic-wrapper-field-16              	  607054	       374.0 ns/op
BenchmarkComponentCopy/immutable.Slice-1000-16              	277799744	         0.8752 ns/op
BenchmarkComponentCopy/immutable.Slice-1000-16              	258687532	         0.9102 ns/op
BenchmarkComponentCopy/immutable.Slice-1000-16              	221118202	         0.9775 ns/op
BenchmarkComponentCopy/array16-16                           	22316606	        10.48 ns/op
BenchmarkComponentCopy/array16-16                           	23946946	        10.50 ns/op
BenchmarkComponentCopy/array16-16                           	22951556	        10.61 ns/op
BenchmarkComponentCopy/array64-16                           	 4949551	        48.10 ns/op
BenchmarkComponentCopy/array64-16                           	 4945212	        48.36 ns/op
BenchmarkComponentCopy/array64-16                           	 4997972	        47.24 ns/op
PASS
ok  	qbench	21.803s
```

## Output: bench-batch-out.txt

```text
goos: linux
goarch: amd64
pkg: qbench
cpu: 11th Gen Intel(R) Core(TM) i9-11900K @ 3.50GHz
BenchmarkEnqueueBatch/immutable-one-Append/n10-16         	 1402646	       168.9 ns/op	     480 B/op	       1 allocs/op
BenchmarkEnqueueBatch/immutable-one-Append/n10-16         	 1405562	       172.9 ns/op	     480 B/op	       1 allocs/op
BenchmarkEnqueueBatch/immutable-10-senders/n10-16         	  257998	       877.4 ns/op	    2672 B/op	      10 allocs/op
BenchmarkEnqueueBatch/immutable-10-senders/n10-16         	  246498	       880.9 ns/op	    2672 B/op	      10 allocs/op
BenchmarkEnqueueBatch/immutable-one-Append/n100-16        	  118449	      1836 ns/op	    5584 B/op	       9 allocs/op
BenchmarkEnqueueBatch/immutable-one-Append/n100-16        	  129116	      1872 ns/op	    5584 B/op	       9 allocs/op
BenchmarkEnqueueBatch/immutable-10-senders/n100-16        	   57184	      4145 ns/op	   13648 B/op	      18 allocs/op
BenchmarkEnqueueBatch/immutable-10-senders/n100-16        	   57262	      4045 ns/op	   13648 B/op	      18 allocs/op
BenchmarkEnqueueBatch/immutable-one-Append/n1000-16       	   11780	     20356 ns/op	   59088 B/op	      93 allocs/op
BenchmarkEnqueueBatch/immutable-one-Append/n1000-16       	   12319	     19590 ns/op	   59088 B/op	      93 allocs/op
BenchmarkEnqueueBatch/immutable-10-senders/n1000-16       	    9568	     22135 ns/op	   64784 B/op	     101 allocs/op
BenchmarkEnqueueBatch/immutable-10-senders/n1000-16       	   10000	     22081 ns/op	   64784 B/op	     101 allocs/op
PASS
ok  	qbench	2.779s
```
