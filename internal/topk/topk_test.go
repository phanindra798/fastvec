package topk

import (
	"math/rand"
	"sort"
	"testing"
)

// brute is the obvious implementation: keep everything, sort it, take k. The
// heap has to agree with it on every input.
func brute(items []Result, k int) []Result {
	out := make([]Result, len(items))
	copy(out, items)
	sort.Slice(out, func(i, j int) bool { return better(out[i], out[j]) })
	if k > len(out) {
		k = len(out)
	}
	return out[:k]
}

func TestMatchesBruteForce(t *testing.T) {
	r := rand.New(rand.NewSource(7))

	for _, n := range []int{0, 1, 5, 100, 1000} {
		for _, k := range []int{1, 3, 10, 50} {
			items := make([]Result, n)
			for i := range items {
				// Few distinct values so ties come up constantly.
				items[i] = Result{ID: int32(i), Dist: float32(r.Intn(20))}
			}
			// Shuffle so insertion order does not match ID order.
			r.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })

			c := New(k)
			for _, it := range items {
				c.Add(it.ID, it.Dist)
			}
			got := c.Results()
			want := brute(items, k)

			if len(got) != len(want) {
				t.Fatalf("n=%d k=%d: got %d results, want %d", n, k, len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("n=%d k=%d: result %d = %+v, want %+v", n, k, i, got[i], want[i])
				}
			}
		}
	}
}

// Same items in a different order must give the same answer. Without the ID
// tie-break this fails as soon as two items share a distance.
func TestOrderIndependent(t *testing.T) {
	r := rand.New(rand.NewSource(8))

	items := make([]Result, 500)
	for i := range items {
		items[i] = Result{ID: int32(i), Dist: float32(r.Intn(10))}
	}

	collect := func(in []Result) []Result {
		c := New(10)
		for _, it := range in {
			c.Add(it.ID, it.Dist)
		}
		return c.Results()
	}

	first := collect(items)
	for trial := 0; trial < 20; trial++ {
		shuffled := make([]Result, len(items))
		copy(shuffled, items)
		r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

		got := collect(shuffled)
		for i := range first {
			if got[i] != first[i] {
				t.Fatalf("shuffle %d changed result %d: %+v vs %+v", trial, i, got[i], first[i])
			}
		}
	}
}

func TestResetKeepsCapacity(t *testing.T) {
	c := New(4)
	for i := 0; i < 100; i++ {
		c.Add(int32(i), float32(i))
	}

	before := cap(c.items)
	c.Reset()

	if c.Len() != 0 {
		t.Errorf("Len after Reset = %d, want 0", c.Len())
	}
	if cap(c.items) != before {
		t.Errorf("Reset dropped the backing array: cap %d, was %d", cap(c.items), before)
	}
}

func TestWorst(t *testing.T) {
	c := New(3)

	if _, full := c.Worst(); full {
		t.Error("empty collector reports itself full")
	}

	c.Add(1, 10)
	c.Add(2, 5)
	if _, full := c.Worst(); full {
		t.Error("collector with 2 of 3 reports itself full")
	}

	c.Add(3, 7)
	d, full := c.Worst()
	if !full {
		t.Fatal("collector with 3 of 3 does not report itself full")
	}
	if d != 10 {
		t.Errorf("Worst = %v, want 10", d)
	}

	c.Add(4, 1)
	if d, _ := c.Worst(); d != 7 {
		t.Errorf("after evicting 10, Worst = %v, want 7", d)
	}
}

func TestResultsIsACopy(t *testing.T) {
	c := New(2)
	c.Add(1, 1)
	c.Add(2, 2)

	got := c.Results()
	c.Reset()
	c.Add(9, 9)

	if got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("Results was invalidated by later use: %+v", got)
	}
}

func BenchmarkAdd(b *testing.B) {
	r := rand.New(rand.NewSource(9))
	dists := make([]float32, 4096)
	for i := range dists {
		dists[i] = r.Float32()
	}

	c := New(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Add(int32(i), dists[i%len(dists)])
	}
}
