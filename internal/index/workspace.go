package index

import "github.com/phanindra798/fastvec/internal/topk"

// workspace is the scratch space one search needs: which nodes it has already
// looked at, plus the two heaps.
//
// Searches run concurrently against a shared graph, so this can't be shared.
// Each search borrows one from a sync.Pool and puts it back, which keeps
// allocation off the query path without any locking.
type workspace struct {
	stamp []uint32 // one per node
	epoch uint32   // this search's marker
	cand  candidates
	res   *topk.Collector
	dist  distCounter
}

func newWorkspace(nodes int) *workspace {
	return &workspace{stamp: make([]uint32, nodes)}
}

// begin readies the workspace for a new search over nodes, keeping whatever
// memory it already had.
func (w *workspace) begin(nodes, ef int) {
	if len(w.stamp) < nodes {
		w.stamp = make([]uint32, nodes)
		w.epoch = 0
	}

	w.epoch++
	if w.epoch == 0 {
		// Wrapped after 4 billion searches. Every stamp would now read as
		// visited, so clear them and start again.
		for i := range w.stamp {
			w.stamp[i] = 0
		}
		w.epoch = 1
	}

	w.cand.reset()
	if w.res == nil || w.res.Cap() != ef {
		w.res = topk.New(ef)
	} else {
		w.res.Reset()
	}
}

func (w *workspace) seen(id int32) bool { return w.stamp[id] == w.epoch }
func (w *workspace) mark(id int32)      { w.stamp[id] = w.epoch }
