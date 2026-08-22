package index

import "github.com/phanindra798/fastvec/internal/topk"

// Index is anything that can answer a nearest neighbour query. Flat implements
// it exactly, HNSW will implement it approximately, and the gate harness and
// benchmarks take either.
//
// efSearch isn't in here. Flat has no equivalent, so it lives on HNSW and
// sweeping it needs a type assertion.
type Index interface {
	Search(q []float32, k int) ([]topk.Result, error)
	Len() int
	Dim() int
}

var _ Index = (*Flat)(nil)
