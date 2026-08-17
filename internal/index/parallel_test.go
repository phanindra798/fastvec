package index

import (
	"fmt"
	"math/rand"
	"testing"
)

// The whole point of the tie-break rule: however the work gets split, the
// answer has to be identical to the single threaded one.
func TestSearchNMatchesSerial(t *testing.T) {
	r := rand.New(rand.NewSource(21))
	f := NewFlat(randomSet(r, 5000, 32))

	for trial := 0; trial < 20; trial++ {
		q := make([]float32, 32)
		for i := range q {
			q[i] = r.Float32()
		}

		want, err := f.Search(q, 10)
		if err != nil {
			t.Fatal(err)
		}

		for _, workers := range []int{1, 2, 3, 4, 7, 16, 64} {
			got, err := f.SearchN(q, 10, workers)
			if err != nil {
				t.Fatal(err)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("workers=%d trial=%d: result %d = %+v, want %+v",
						workers, trial, i, got[i], want[i])
				}
			}
		}
	}
}

// Lots of repeated vectors, so nearly every distance ties.
//
// Weaker than it looks: contiguous ranges merged in worker order already give a
// stable answer even without the ID tie-break, which I found by deleting the
// rule and watching this still pass. It does catch a merge that depends on
// which worker finished first, which is what it's here for.
func TestSearchNDeterministicWithTies(t *testing.T) {
	const distinct = 40

	set := randomSet(rand.New(rand.NewSource(22)), 200, 4)
	// Copy the first 40 over the rest, so every vector appears five times and
	// nearly every distance is a tie.
	for i := distinct; i < set.Len(); i++ {
		copy(set.At(i), set.At(i%distinct))
	}
	f := NewFlat(set)

	// Guard against the flatten loop silently doing nothing, which is exactly
	// what happened the first time I wrote this.
	if set.Len() <= distinct {
		t.Fatalf("set has %d vectors, need more than %d for duplicates", set.Len(), distinct)
	}

	q := set.At(0)
	first, err := f.SearchN(q, 5, 8)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		got, _ := f.SearchN(q, 5, 8)
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("run %d differs at %d: %+v vs %+v", i, j, got[j], first[j])
			}
		}
	}
}

// Uneven splits: 5000 across 7 workers leaves a remainder, and nothing should
// get skipped or scanned twice.
func TestSearchNCoversEveryVector(t *testing.T) {
	r := rand.New(rand.NewSource(23))
	set := randomSet(r, 5000, 8)
	f := NewFlat(set)

	for _, workers := range []int{3, 7, 13, 16} {
		// Every vector is its own nearest neighbour at distance 0.
		for _, id := range []int{0, 1, 2499, 4998, 4999} {
			got, err := f.SearchN(set.At(id), 1, workers)
			if err != nil {
				t.Fatal(err)
			}
			if got[0].Dist != 0 {
				t.Errorf("workers=%d: vector %d not found, nearest was %+v", workers, id, got[0])
			}
		}
	}
}

func BenchmarkFlatSearchN(b *testing.B) {
	r := rand.New(rand.NewSource(24))
	f := NewFlat(randomSet(r, 100_000, 128))

	q := make([]float32, 128)
	for i := range q {
		q[i] = r.Float32()
	}

	for _, workers := range []int{1, 2, 4, 6, 8, 12, 16} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := f.SearchN(q, 10, workers); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
