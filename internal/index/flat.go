// Package index holds the search structures. Flat compares the query against
// every stored vector, so it's slow but exact. It produces the ground truth
// HNSW's recall gets measured against, and it's the baseline to beat.
package index

import (
	"fmt"
	"sync"

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
func (f *Flat) Search(q []float32, k int) ([]topk.Result, error) {
	if err := f.check(q, k); err != nil {
		return nil, err
	}

	c := topk.New(k)
	f.scan(q, 0, f.Len(), c)
	return c.Results(), nil
}

// SearchN splits the scan across workers goroutines. Anything below 2 falls
// back to Search.
//
// Each worker keeps its own collector so nothing is shared during the scan,
// then the k results from each get merged through one more collector. That's
// workers*k items, nothing next to the scan itself.
//
// Results go into partials indexed by worker, so the merge always reads them in
// the same order however the goroutines finish. Which is why this returns
// exactly what Search returns.
func (f *Flat) SearchN(q []float32, k, workers int) ([]topk.Result, error) {
	if err := f.check(q, k); err != nil {
		return nil, err
	}

	n := f.Len()
	if workers < 2 || n < workers {
		return f.Search(q, k)
	}

	partials := make([][]topk.Result, workers)
	chunk, extra := n/workers, n%workers

	var wg sync.WaitGroup
	start := 0
	for w := 0; w < workers; w++ {
		size := chunk
		if w < extra {
			size++ // spread the remainder over the first few workers
		}
		lo, hi := start, start+size
		start = hi

		wg.Add(1)
		go func() {
			defer wg.Done()
			c := topk.New(k)
			f.scan(q, lo, hi, c)
			partials[w] = c.Results()
		}()
	}
	wg.Wait()

	merged := topk.New(k)
	for _, part := range partials {
		for _, r := range part {
			merged.Add(r.ID, r.Dist)
		}
	}
	return merged.Results(), nil
}

func (f *Flat) scan(q []float32, lo, hi int, c *topk.Collector) {
	for i := lo; i < hi; i++ {
		v := f.data[i*f.dim : (i+1)*f.dim : (i+1)*f.dim]
		c.Add(int32(i), distance.L2Squared(q, v))
	}
}

func (f *Flat) check(q []float32, k int) error {
	if len(q) != f.dim {
		return fmt.Errorf("query has dimension %d, index has %d", len(q), f.dim)
	}
	if k < 1 {
		return fmt.Errorf("k must be at least 1, got %d", k)
	}
	return nil
}
