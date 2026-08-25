package index

import (
	"fmt"
	"sync"

	"github.com/phanindra798/fastvec/internal/distance"
	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/topk"
)

// HNSW is a proximity graph over the vectors: every node keeps a short list of
// nearby nodes, and a query walks the graph towards whatever is closest.
//
// Stage 1: one flat graph, no hierarchy, nearest-M neighbour choice. Layers
// come in stage 2, the selection heuristic in stage 3, so each can be measured
// separately.
type HNSW struct {
	dim  int
	data []float32

	// neighbours[i] holds the IDs of node i's neighbours, capped at mMax.
	neighbours  [][]int32
	m           int
	mMax        int
	efConstruct int
	efSearch    int

	entry int32

	pool sync.Pool
}

type Params struct {
	M           int // neighbours chosen for a new node
	MMax        int // cap before an existing list gets pruned. 0 means 2*M
	EfConstruct int // candidate list size while building
	EfSearch    int // candidate list size at query time, the accuracy dial
}

func DefaultParams() Params {
	return Params{M: 16, MMax: 32, EfConstruct: 200, EfSearch: 100}
}

// BuildHNSW inserts every vector in set, one at a time, in ID order.
func BuildHNSW(set *fvecs.Float, p Params) (*HNSW, error) {
	// Node 0 is the entry point, so with nothing to index every search would
	// read a vector that isn't there.
	if set.Dim < 1 || set.Len() < 1 {
		return nil, fmt.Errorf("need at least one vector, got %d of dimension %d", set.Len(), set.Dim)
	}
	if p.M < 1 {
		return nil, fmt.Errorf("M must be at least 1, got %d", p.M)
	}
	if p.EfConstruct < p.M {
		return nil, fmt.Errorf("efConstruction (%d) must be at least M (%d)", p.EfConstruct, p.M)
	}
	if p.MMax == 0 {
		p.MMax = 2 * p.M
	}
	if p.MMax < p.M {
		return nil, fmt.Errorf("MMax (%d) must be at least M (%d)", p.MMax, p.M)
	}

	n := set.Len()
	h := &HNSW{
		dim:         set.Dim,
		data:        set.Data,
		neighbours:  make([][]int32, n),
		m:           p.M,
		mMax:        p.MMax,
		efConstruct: p.EfConstruct,
		efSearch:    p.EfSearch,
		entry:       0,
	}
	h.pool.New = func() any { return newWorkspace(n) }

	for i := 1; i < n; i++ {
		h.insert(int32(i))
	}
	return h, nil
}

func (h *HNSW) Dim() int { return h.dim }
func (h *HNSW) Len() int { return len(h.neighbours) }

// SetEfSearch changes the accuracy/speed dial. Higher keeps more candidates
// alive during a walk, which finds more of the true neighbours and costs more
// distance computations.
func (h *HNSW) SetEfSearch(ef int) { h.efSearch = ef }

func (h *HNSW) EfSearch() int { return h.efSearch }

func (h *HNSW) vec(id int32) []float32 {
	lo := int(id) * h.dim
	return h.data[lo : lo+h.dim : lo+h.dim]
}

func (h *HNSW) distTo(q []float32, id int32) float32 {
	return distance.L2Squared(q, h.vec(id))
}

// insert adds node id to the graph: find where it belongs, link to the nearest
// few, and let them link back.
func (h *HNSW) insert(id int32) {
	q := h.vec(id)

	found := h.searchGraph(q, h.efConstruct, id)

	// Stage 1 keeps the nearest M. Stage 3 replaces this with the paper's
	// heuristic, which keeps some further-away nodes because they point
	// somewhere no closer neighbour does.
	if len(found) > h.m {
		found = found[:h.m]
	}

	h.neighbours[id] = make([]int32, 0, h.m)
	for _, r := range found {
		h.neighbours[id] = append(h.neighbours[id], r.ID)
		h.link(r.ID, id)
	}
}

// link adds id to other's neighbour list, pruning back to mMax if that pushes
// it over. Without a cap, early nodes accumulate thousands of edges and every
// walk through them scans the lot.
//
// Prune at mMax, not m. Pruning at m orphaned 23% of a 100k graph, see
// decisions.md.
func (h *HNSW) link(other, id int32) {
	h.neighbours[other] = append(h.neighbours[other], id)
	if len(h.neighbours[other]) <= h.mMax {
		return
	}

	v := h.vec(other)
	keep := topk.New(h.mMax)
	for _, nb := range h.neighbours[other] {
		keep.Add(nb, distance.L2Squared(v, h.vec(nb)))
	}

	trimmed := h.neighbours[other][:0]
	for _, r := range keep.Results() {
		trimmed = append(trimmed, r.ID)
	}
	h.neighbours[other] = trimmed
}

// searchGraph walks the graph towards q and returns the best ef nodes it saw,
// nearest first.
//
// skip is a node to leave out of the results, used while building so a vector
// doesn't become its own neighbour. Pass -1 when querying.
func (h *HNSW) searchGraph(q []float32, ef int, skip int32) []topk.Result {
	w := h.pool.Get().(*workspace)
	defer h.pool.Put(w)

	w.begin(h.Len(), ef)

	start := topk.Result{ID: h.entry, Dist: h.distTo(q, h.entry)}
	w.mark(h.entry)
	w.cand.push(start)
	if h.entry != skip {
		w.res.Add(start.ID, start.Dist)
	}

	for w.cand.len() > 0 {
		c := w.cand.pop()

		// Everything left to explore is further away than the worst result
		// already held, and the results are full. Nothing reachable from here
		// can improve the answer.
		if worst, full := w.res.Worst(); full && c.Dist > worst {
			break
		}

		for _, nb := range h.neighbours[c.ID] {
			if w.seen(nb) {
				continue
			}
			w.mark(nb)

			d := h.distTo(q, nb)
			worst, full := w.res.Worst()
			if full && d >= worst {
				continue
			}

			w.cand.push(topk.Result{ID: nb, Dist: d})
			if nb != skip {
				w.res.Add(nb, d)
			}
		}
	}

	return w.res.Results()
}

func (h *HNSW) Search(q []float32, k int) ([]topk.Result, error) {
	if len(q) != h.dim {
		return nil, fmt.Errorf("query has dimension %d, index has %d", len(q), h.dim)
	}
	if k < 1 {
		return nil, fmt.Errorf("k must be at least 1, got %d", k)
	}

	ef := h.efSearch
	if ef < k {
		ef = k
	}

	res := h.searchGraph(q, ef, -1)
	if len(res) > k {
		res = res[:k]
	}
	return res, nil
}

// Reachable counts the nodes a walk can get to from the entry point, ignoring
// distances. Anything unreachable can never be returned by a search however
// high efSearch goes, and that is a build bug rather than a search bug.
func (h *HNSW) Reachable() int {
	seen := make([]bool, h.Len())
	seen[h.entry] = true
	queue := []int32{h.entry}
	count := 1

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, nb := range h.neighbours[node] {
			if !seen[nb] {
				seen[nb] = true
				count++
				queue = append(queue, nb)
			}
		}
	}
	return count
}

// Degrees returns the smallest, largest and mean neighbour count. A max far
// above M means the trim in link is not firing.
func (h *HNSW) Degrees() (min, max int, mean float64) {
	min = -1
	total := 0
	for _, nb := range h.neighbours {
		d := len(nb)
		if min < 0 || d < min {
			min = d
		}
		if d > max {
			max = d
		}
		total += d
	}
	if min < 0 {
		min = 0
	}
	return min, max, float64(total) / float64(h.Len())
}

var _ Index = (*HNSW)(nil)
