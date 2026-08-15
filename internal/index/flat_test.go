package index

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/phanindra798/fastvec/internal/distance"
	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/topk"
)

func randomSet(r *rand.Rand, n, dim int) *fvecs.Float {
	set := &fvecs.Float{Dim: dim, Data: make([]float32, n*dim)}
	for i := range set.Data {
		set.Data[i] = r.Float32()
	}
	return set
}

func TestSearchKnownAnswer(t *testing.T) {
	set := &fvecs.Float{
		Dim: 2,
		Data: []float32{
			0, 0, // id 0
			10, 10, // id 1
			1, 1, // id 2
			5, 5, // id 3
		},
	}

	got, err := NewFlat(set).Search([]float32{0, 0}, 3)
	if err != nil {
		t.Fatal(err)
	}

	want := []int32{0, 2, 3}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("result %d has ID %d, want %d (full: %+v)", i, got[i].ID, id, got)
		}
	}
	if got[0].Dist != 0 {
		t.Errorf("nearest distance = %v, want 0", got[0].Dist)
	}
}

// The real check: against a version so simple it cannot be subtly wrong.
func TestSearchMatchesNaiveScan(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	set := randomSet(r, 500, 16)
	f := NewFlat(set)

	for trial := 0; trial < 50; trial++ {
		q := make([]float32, 16)
		for i := range q {
			q[i] = r.Float32()
		}

		all := make([]topk.Result, set.Len())
		for i := range all {
			all[i] = topk.Result{ID: int32(i), Dist: distance.L2Squared(q, set.At(i))}
		}
		sort.Slice(all, func(i, j int) bool {
			if all[i].Dist != all[j].Dist {
				return all[i].Dist < all[j].Dist
			}
			return all[i].ID < all[j].ID
		})

		got, err := f.Search(q, 10)
		if err != nil {
			t.Fatal(err)
		}
		for i := range got {
			if got[i] != all[i] {
				t.Fatalf("trial %d result %d = %+v, want %+v", trial, i, got[i], all[i])
			}
		}
	}
}

func TestSearchKLargerThanIndex(t *testing.T) {
	set := &fvecs.Float{Dim: 1, Data: []float32{1, 2, 3}}

	got, err := NewFlat(set).Search([]float32{0}, 100)
	if err != nil {
		t.Fatalf("asking for more than the index holds should not be an error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d results, want 3", len(got))
	}
}

func TestSearchRejectsBadInput(t *testing.T) {
	f := NewFlat(&fvecs.Float{Dim: 3, Data: []float32{1, 2, 3}})

	if _, err := f.Search([]float32{1, 2}, 1); err == nil {
		t.Error("wrong query dimension was accepted")
	}
	if _, err := f.Search([]float32{1, 2, 3}, 0); err == nil {
		t.Error("k of 0 was accepted")
	}
}

// Repeated vectors mean repeated distances. The IDs returned must not depend on
// scan order, or recall numbers stop being comparable between runs.
func TestSearchIsDeterministicWithTies(t *testing.T) {
	set := &fvecs.Float{Dim: 1, Data: []float32{5, 5, 5, 5, 5, 5}}
	f := NewFlat(set)

	first, err := f.Search([]float32{0}, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		got, _ := f.Search([]float32{0}, 3)
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("run %d differs at %d: %+v vs %+v", i, j, got[j], first[j])
			}
		}
	}
	// With every distance equal, the tie-break should hand back the lowest IDs.
	for i, r := range first {
		if r.ID != int32(i) {
			t.Errorf("result %d has ID %d, want %d", i, r.ID, i)
		}
	}
}

func BenchmarkFlatSearch(b *testing.B) {
	r := rand.New(rand.NewSource(12))
	f := NewFlat(randomSet(r, 100_000, 128))

	q := make([]float32, 128)
	for i := range q {
		q[i] = r.Float32()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.Search(q, 10); err != nil {
			b.Fatal(err)
		}
	}
}
