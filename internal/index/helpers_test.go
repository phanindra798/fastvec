package index

import (
	"math/rand"
	"testing"

	"github.com/phanindra798/fastvec/internal/distance"
	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/topk"
)

func smallSet(t *testing.T, rows [][]float32) *fvecs.Float {
	t.Helper()

	dim := len(rows[0])
	set := &fvecs.Float{Dim: dim, Data: make([]float32, 0, dim*len(rows))}
	for _, r := range rows {
		if len(r) != dim {
			t.Fatalf("row has %d values, first row had %d", len(r), dim)
		}
		set.Data = append(set.Data, r...)
	}
	return set
}

// clumps builds groups of points far enough apart that nothing in one is near
// anything in another. Nearest-M selection links inside a clump and never
// across, which is the failure the heuristic is for.
func clumps(r *rand.Rand, perClump, dim, count int, spread float32) *fvecs.Float {
	set := &fvecs.Float{Dim: dim, Data: make([]float32, 0, perClump*count*dim)}

	for c := 0; c < count; c++ {
		centre := make([]float32, dim)
		for i := range centre {
			centre[i] = float32(c) * spread
		}
		for p := 0; p < perClump; p++ {
			for i := 0; i < dim; i++ {
				set.Data = append(set.Data, centre[i]+r.Float32())
			}
		}
	}
	return set
}

// rank orders ids by distance from node, nearest first, which is the shape
// selectNeighbours expects.
func (h *HNSW) rank(node int32, ids []int32) []topk.Result {
	v := h.vec(node)
	c := topk.New(len(ids))
	for _, id := range ids {
		c.Add(id, distance.L2Squared(v, h.vec(id)))
	}
	return c.Results()
}
