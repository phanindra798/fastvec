package index

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/phanindra798/fastvec/internal/distance"
	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/topk"
)

// Always runs, including -short and CI without SIFT. Queries are generated
// independently of base vectors so finding a stored vector itself cannot pass
// as evidence of general nearest-neighbour quality.
func TestHNSWSyntheticRecall(t *testing.T) {
	const k = 10
	for _, kind := range []string{"uniform", "clusters", "duplicates"} {
		for _, seed := range []int64{81, 82, 83} {
			t.Run(fmt.Sprintf("%s/seed=%d", kind, seed), func(t *testing.T) {
				base, queries := syntheticRecallData(kind, seed)
				flat := NewFlat(base)
				thresholds := make([]float32, queries.Len())
				for q := range thresholds {
					truth, err := flat.Search(queries.At(q), k)
					if err != nil {
						t.Fatal(err)
					}
					thresholds[q] = truth[k-1].Dist
				}
				for _, workers := range []int{1, 4} {
					t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
						p := DefaultParams()
						p.Seed, p.BuildWorkers = seed, workers
						p.EfConstruct, p.EfSearch = 100, 100
						h, err := BuildHNSW(base, p)
						if err != nil {
							t.Fatal(err)
						}
						hits := 0
						for q, threshold := range thresholds {
							got, err := h.Search(queries.At(q), k)
							if err != nil {
								t.Fatal(err)
							}
							n, err := syntheticHits(base, queries.At(q), got, k, threshold)
							if err != nil {
								t.Fatalf("query %d: %v", q, err)
							}
							hits += n
						}
						recall := float64(hits) / float64(queries.Len()*k)
						// Fixed regression floors, not claims about arbitrary datasets.
						// Parallel builds need scheduling headroom, but must independently
						// meet a quality floor against exact search.
						floor := 0.95
						if workers > 1 {
							floor = 0.90
						}
						t.Logf("recall@%d=%.4f, required %.2f", k, recall, floor)
						if recall < floor {
							t.Fatalf("recall %.4f below %.2f", recall, floor)
						}
					})
				}
			})
		}
	}
}

func syntheticRecallData(kind string, seed int64) (*fvecs.Float, *fvecs.Float) {
	r := rand.New(rand.NewSource(seed))
	const dim, count, queryCount = 16, 2000, 120
	base := randomSet(r, count, dim)
	queries := randomSet(r, queryCount, dim)
	if kind == "clusters" {
		// Insert cluster-by-cluster. Independent queries include some between
		// cluster centres, exercising graph navigation beyond self matches.
		for i := 0; i < count; i++ {
			for d := range base.At(i) {
				base.At(i)[d] += float32(i/500) * 4
			}
		}
		for i := 0; i < queryCount; i++ {
			centre := float32(i%4) * 4
			if i%5 == 0 && i%4 < 3 {
				centre += 2
			}
			for d := range queries.At(i) {
				queries.At(i)[d] += centre
			}
		}
	}
	if kind == "duplicates" {
		for i := count / 2; i < count; i++ {
			copy(base.At(i), base.At(i-count/2))
		}
	}
	return base, queries
}

// Recompute distances from IDs rather than trusting the index's reported
// distances. Equal-distance alternative IDs are valid, repeated IDs are not.
func syntheticHits(base *fvecs.Float, q []float32, got []topk.Result, k int, threshold float32) (int, error) {
	if len(got) != k {
		return 0, fmt.Errorf("got %d results, want %d", len(got), k)
	}
	seen := make(map[int32]bool, k)
	hits := 0
	for _, result := range got {
		if result.ID < 0 || int(result.ID) >= base.Len() || seen[result.ID] {
			return 0, fmt.Errorf("invalid or repeated result ID %d", result.ID)
		}
		seen[result.ID] = true
		if distance.L2Squared(q, base.At(int(result.ID))) <= threshold {
			hits++
		}
	}
	return hits, nil
}

func TestSyntheticRecallScoring(t *testing.T) {
	base := &fvecs.Float{Dim: 1, Data: []float32{0, 0, 10}}
	for _, tc := range []struct {
		name string
		got  []topk.Result
		hits int
		bad  bool
	}{
		{"alternate tied ID", []topk.Result{{ID: 1}}, 1, false},
		{"false distance", []topk.Result{{ID: 2, Dist: 0}}, 0, false},
		{"missing result", nil, 0, true},
		{"invalid ID", []topk.Result{{ID: -1}}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits, err := syntheticHits(base, []float32{0}, tc.got, 1, 0)
			if hits != tc.hits || (err != nil) != tc.bad {
				t.Fatalf("hits=%d err=%v", hits, err)
			}
		})
	}
	if _, err := syntheticHits(base, []float32{0}, []topk.Result{{ID: 0}, {ID: 0}}, 2, 0); err == nil {
		t.Fatal("repeated IDs counted as separate neighbours")
	}
}
