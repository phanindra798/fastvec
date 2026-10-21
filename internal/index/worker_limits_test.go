package index

import (
	"sync"
	"testing"

	"github.com/phanindra798/fastvec/internal/fvecs"
)

func TestSetOwnPreservesReverseLinks(t *testing.T) {
	h := &HNSW{
		dim: 1, data: []float32{0, 1, 2}, mMax0: 2, building: true,
		links: [][][]int32{{{2}}, {{}}, {{}}},
		locks: make([]sync.Mutex, 3),
	}
	h.setOwn(0, 0, []int32{1})
	got := h.links[0][0]
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("lost reverse link: %v", got)
	}
	h.setOwn(0, 0, []int32{1, 2})
	if len(h.links[0][0]) != 2 {
		t.Fatalf("duplicate reverse links: %v", h.links[0][0])
	}
}

func TestBuildWithExcessiveWorkers(t *testing.T) {
	set := &fvecs.Float{Dim: 1, Data: []float32{0, 1, 2}}
	p := DefaultParams()
	p.BuildWorkers = int(^uint(0) >> 1)
	h, err := BuildHNSW(set, p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.Search([]float32{1}, 1)
	if err != nil || len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("search after excessive worker request: %v, %v", got, err)
	}
}
