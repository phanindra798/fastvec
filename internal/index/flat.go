// Package index holds the search structures. Flat compares the query against
// every stored vector, so it's slow but exact. It produces the ground truth
// HNSW's recall gets measured against, and it's the baseline to beat.
package index

import (
	"fmt"

	"github.com/phanindra798/fastvec/internal/distance"
	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/topk"
)

type Flat struct {
	dim  int
	data []float32
}

func NewFlat(set *fvecs.Float) *Flat {
	return &Flat{dim: set.Dim, data: set.Data}
}

func (f *Flat) Dim() int { return f.dim }

func (f *Flat) Len() int { return len(f.data) / f.dim }

// Search returns the k nearest vectors to q, nearest first. Asking for more
// than the index holds just returns everything.
//
// TODO: split the scan across goroutines. 8.4 ms per query at 100k means the
// ground truth pass over the full million is going to take a while.
func (f *Flat) Search(q []float32, k int) ([]topk.Result, error) {
	if len(q) != f.dim {
		return nil, fmt.Errorf("query has dimension %d, index has %d", len(q), f.dim)
	}
	if k < 1 {
		return nil, fmt.Errorf("k must be at least 1, got %d", k)
	}

	c := topk.New(k)
	for i, n := 0, f.Len(); i < n; i++ {
		v := f.data[i*f.dim : (i+1)*f.dim : (i+1)*f.dim]
		c.Add(int32(i), distance.L2Squared(q, v))
	}
	return c.Results(), nil
}
