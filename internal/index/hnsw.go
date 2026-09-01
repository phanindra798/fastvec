package index

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/phanindra798/fastvec/internal/distance"
	"github.com/phanindra798/fastvec/internal/fvecs"
	"github.com/phanindra798/fastvec/internal/topk"
)

// HNSW is a stack of proximity graphs over the vectors. Every node is in the
// bottom layer; each layer above holds a shrinking random subset, so its edges
// span longer distances.
//
// A query enters at the top, walks greedily until no neighbour is closer, drops
// a layer and repeats. The sparse upper layers cross the space in a few hops,
// the bottom one does the fine-grained work.
//
// Stage 2. Neighbour selection is still nearest-M; the heuristic is stage 3.
type HNSW struct {
	dim  int
	data []float32

	// links[node][level] holds that node's neighbours at that level. A node
	// present only at the bottom has one entry.
	links [][][]int32

	m     int // neighbours chosen for a new node
	mMax  int // cap on a list above level 0
	mMax0 int // cap at level 0, where every node lives
	ml    float64

	efConstruct int
	singleLayer bool
	nearestM    bool

	// Tunable while queries are running, so it can't be a plain int.
	efSearch atomic.Int64

	entry    int32
	maxLevel int
	seed     int64

	// Only meaningful while building. A finished index is immutable, so
	// queries never take a lock and pay nothing for these.
	locks    []sync.Mutex
	topMu    sync.Mutex
	building bool

	rng  *rand.Rand // build only, never touched by a query
	pool sync.Pool

	distTotal atomic.Uint64
}

type Params struct {
	M           int   // neighbours chosen for a new node
	MMax        int   // cap above level 0. 0 means M
	MMax0       int   // cap at level 0. 0 means 2*M
	EfConstruct int   // candidate list size while building
	EfSearch    int   // candidate list size at query time, the accuracy dial
	Seed        int64 // level assignment is random, so this pins the build

	// SingleLayer forces every node to level 0, which is what the index was
	// before the hierarchy. Kept so the two can be measured side by side
	// instead of me claiming layers helped.
	SingleLayer bool

	// NearestM picks neighbours by plain distance instead of the paper's
	// heuristic. Same reason: the two need comparing, not asserting.
	NearestM bool

	// BuildWorkers inserts across this many goroutines. Below 2 runs
	// sequentially, which stays the default because it is the only mode that
	// produces the same index twice. See decisions.md.
	BuildWorkers int
}

func DefaultParams() Params {
	return Params{M: 16, MMax: 16, MMax0: 32, EfConstruct: 200, EfSearch: 100, Seed: 42}
}

// BuildHNSW inserts every vector in set, one at a time, in ID order.
func BuildHNSW(set *fvecs.Float, p Params) (*HNSW, error) {
	// Node 0 is the first entry point, so with nothing to index every search
	// would read a vector that isn't there.
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
		p.MMax = p.M
	}
	if p.MMax0 == 0 {
		p.MMax0 = 2 * p.M
	}
	if p.MMax < p.M || p.MMax0 < p.M {
		return nil, fmt.Errorf("MMax (%d) and MMax0 (%d) must both be at least M (%d)", p.MMax, p.MMax0, p.M)
	}

	n := set.Len()
	h := &HNSW{
		dim:         set.Dim,
		data:        set.Data,
		links:       make([][][]int32, n),
		m:           p.M,
		mMax:        p.MMax,
		mMax0:       p.MMax0,
		ml:          1 / math.Log(float64(p.M)),
		efConstruct: p.EfConstruct,
		singleLayer: p.SingleLayer,
		nearestM:    p.NearestM,
		seed:        p.Seed,
		rng:         rand.New(rand.NewSource(p.Seed)),
	}
	h.efSearch.Store(int64(p.EfSearch))
	h.pool.New = func() any { return newWorkspace(n) }

	// Node 0 seeds the top layer. Whatever level it draws becomes the height of
	// the index until something taller turns up.
	h.links[0] = newLevels(h.randomLevel())
	h.entry = 0
	h.maxLevel = len(h.links[0]) - 1

	if p.BuildWorkers > 1 {
		h.buildParallel(n, p.BuildWorkers)
	} else {
		for i := 1; i < n; i++ {
			h.insert(int32(i))
		}
	}
	return h, nil
}

// buildParallel inserts across several goroutines.
//
// Levels are drawn up front on one goroutine, because the RNG is stateful and
// having workers pull from it would make the level assignment depend on
// scheduling. Drawing them in ID order keeps that part identical to a
// sequential build; what varies is the order nodes get linked in.
//
// Each node gets a mutex covering its neighbour lists. Traversal copies a list
// under that lock rather than reading it live, since another worker may be
// appending. Contention is low: a million nodes against a handful of workers.
func (h *HNSW) buildParallel(n, workers int) {
	levels := make([]int, n)
	for i := 1; i < n; i++ {
		levels[i] = h.randomLevel()
	}

	h.locks = make([]sync.Mutex, n)
	h.building = true
	defer func() {
		h.building = false
		h.locks = nil
	}()

	ids := make(chan int32, 1024)
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range ids {
				h.insertAt(id, levels[id])
			}
		}()
	}

	for i := 1; i < n; i++ {
		ids <- int32(i)
	}
	close(ids)
	wg.Wait()
}

func newLevels(top int) [][]int32 {
	return make([][]int32, top+1)
}

// randomLevel draws a node's top level. The distribution is geometric with
// factor 1/M, so each layer up holds roughly 1/M of the one below and the stack
// is only a few deep even for millions of vectors.
func (h *HNSW) randomLevel() int {
	if h.singleLayer {
		return 0
	}
	return int(-math.Log(1-h.rng.Float64()) * h.ml)
}

func (h *HNSW) Dim() int      { return h.dim }
func (h *HNSW) Len() int      { return len(h.links) }
func (h *HNSW) MaxLevel() int { return h.maxLevel }

// SetEfSearch changes the accuracy/speed dial. Higher keeps more candidates
// alive during the bottom-layer walk, finding more of the true neighbours and
// computing more distances.
//
// Safe to call while searches are running. Each query reads it once, so a
// change lands on the next query rather than halfway through one.
func (h *HNSW) SetEfSearch(ef int) { h.efSearch.Store(int64(ef)) }
func (h *HNSW) EfSearch() int      { return int(h.efSearch.Load()) }

// Distances returns how many distance computations every search so far has
// done. Always zero unless built with -tags diagnostic.
func (h *HNSW) Distances() uint64 { return h.distTotal.Load() }
func (h *HNSW) ResetDistances()   { h.distTotal.Store(0) }

func (h *HNSW) vec(id int32) []float32 {
	lo := int(id) * h.dim
	return h.data[lo : lo+h.dim : lo+h.dim]
}

func (h *HNSW) levelOf(id int32) int { return len(h.links[id]) - 1 }

func (h *HNSW) capAt(level int) int {
	if level == 0 {
		return h.mMax0
	}
	return h.mMax
}

// insert places a node at a random top level, walks down to it, then links it
// into every layer from there to the bottom.
func (h *HNSW) insert(id int32) {
	h.insertAt(id, h.randomLevel())
}

// insertAt is insert with the level already chosen. Parallel builds draw all
// the levels up front, so the RNG stays on one goroutine.
func (h *HNSW) insertAt(id int32, level int) {
	q := h.vec(id)

	// Publishing this before anything links to id is what makes the node safe
	// to follow. A worker only sees id in some other node's list after taking
	// that node's lock, and the lock release orders this write ahead of it.
	h.links[id] = newLevels(level)

	w := h.pool.Get().(*workspace)
	defer h.pool.Put(w)

	entry, top := h.topOfIndex()
	ep := entry

	// Above the new node's own level there is nothing to link, so just find the
	// best entry point to carry down. ef of 1 is a plain greedy walk.
	for lc := top; lc > level; lc-- {
		ep = h.descend(q, ep, lc, w)
	}

	start := level
	if start > top {
		start = top
	}

	for lc := start; lc >= 0; lc-- {
		found := h.searchLayer(q, ep, h.efConstruct, lc, id, w)
		if len(found) == 0 {
			continue
		}

		keep := h.selectNeighbours(id, found, h.m)

		own := make([]int32, 0, h.m)
		for _, r := range keep {
			own = append(own, r.ID)
		}
		h.setOwn(id, lc, own)

		for _, r := range keep {
			h.link(r.ID, id, lc)
		}

		ep = found[0].ID
	}

	h.raiseTop(id, level)
}

// topOfIndex reads the entry point and its level together, so a parallel build
// cannot descend from an entry that has since been replaced by a taller node.
func (h *HNSW) topOfIndex() (int32, int) {
	if !h.building {
		return h.entry, h.maxLevel
	}
	h.topMu.Lock()
	defer h.topMu.Unlock()
	return h.entry, h.maxLevel
}

func (h *HNSW) raiseTop(id int32, level int) {
	if !h.building {
		if level > h.maxLevel {
			h.maxLevel = level
			h.entry = id
		}
		return
	}
	h.topMu.Lock()
	if level > h.maxLevel {
		h.maxLevel = level
		h.entry = id
	}
	h.topMu.Unlock()
}

func (h *HNSW) setOwn(id int32, level int, nbs []int32) {
	if !h.building {
		h.links[id][level] = nbs
		return
	}
	h.locks[id].Lock()
	h.links[id][level] = nbs
	h.locks[id].Unlock()
}

// neighbours hands back node id's list at this level.
//
// A finished index is immutable, so a query gets the slice directly and pays
// nothing. During a build another worker may be appending to it, so the list is
// copied into the caller's scratch buffer under the node's lock.
func (h *HNSW) neighbours(id int32, level int, scratch []int32) []int32 {
	if !h.building {
		return h.links[id][level]
	}
	h.locks[id].Lock()
	scratch = append(scratch[:0], h.links[id][level]...)
	h.locks[id].Unlock()
	return scratch
}

// link adds id to other's neighbour list at this level, pruning back to the
// level's cap if that pushes it over.
//
// Prune at mMax, not m. Pruning at m orphaned 23% of a 100k graph, see
// decisions.md.
// The whole append-and-maybe-prune runs under other's lock during a parallel
// build. Splitting it would let two workers each read a list of mMax, each
// decide to prune, and each write back a result missing the other's edge.
func (h *HNSW) link(other, id int32, level int) {
	if h.building {
		h.locks[other].Lock()
		defer h.locks[other].Unlock()
	}

	h.links[other][level] = append(h.links[other][level], id)

	limit := h.capAt(level)
	if len(h.links[other][level]) <= limit {
		return
	}

	v := h.vec(other)
	ranked := topk.New(len(h.links[other][level]))
	for _, nb := range h.links[other][level] {
		ranked.Add(nb, distance.L2Squared(v, h.vec(nb)))
	}

	keep := h.selectNeighbours(other, ranked.Results(), limit)

	trimmed := h.links[other][level][:0]
	for _, r := range keep {
		trimmed = append(trimmed, r.ID)
	}
	h.links[other][level] = trimmed
}

// selectNeighbours picks which m of the candidates to keep as neighbours of
// node. Candidates arrive sorted nearest first.
//
// The obvious answer is the nearest m, and it builds a graph that falls into
// clusters: inside a dense group every node links only to others in the same
// group, and a walk that enters can't get out. Measured at 100k, that split the
// bottom layer into six pieces with the largest holding 54% of the nodes.
//
// The paper's heuristic instead keeps a candidate only when it is closer to
// node than to any neighbour already selected. A candidate sitting behind one
// that's already in gets dropped, however near it is, because the existing one
// already covers that direction. What survives are neighbours pointing
// different ways, including some further off, and those are the edges that let
// a walk leave a cluster.
//
// Costs m^2 distance computations per call, which shows up in build time.
func (h *HNSW) selectNeighbours(node int32, candidates []topk.Result, m int) []topk.Result {
	if h.nearestM {
		if len(candidates) > m {
			return candidates[:m]
		}
		return candidates
	}

	kept := make([]topk.Result, 0, m)
	for _, c := range candidates {
		if len(kept) >= m {
			break
		}

		covered := false
		for _, k := range kept {
			if distance.L2Squared(h.vec(c.ID), h.vec(k.ID)) < c.Dist {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, c)
		}
	}
	return kept
}

// descend does a greedy walk at one level and returns where it stopped. Used
// for the layers above the target, where only the endpoint matters.
func (h *HNSW) descend(q []float32, ep int32, level int, w *workspace) int32 {
	best := ep
	bestDist := h.measure(q, ep, w)

	for improved := true; improved; {
		improved = false
		w.nbuf = h.neighbours(best, level, w.nbuf)
		for _, nb := range w.nbuf {
			if d := h.measure(q, nb, w); d < bestDist {
				best, bestDist, improved = nb, d, true
			}
		}
	}
	return best
}

// searchLayer is best-first search within one level, keeping the best ef nodes
// seen. Everything else in the index calls it.
//
// skip leaves one node out of the results, used while building so a vector
// doesn't become its own neighbour. Pass -1 when querying.
func (h *HNSW) searchLayer(q []float32, ep int32, ef, level int, skip int32, w *workspace) []topk.Result {
	w.begin(h.Len(), ef)

	d := h.measure(q, ep, w)
	w.mark(ep)
	w.cand.push(topk.Result{ID: ep, Dist: d})
	if ep != skip {
		w.res.Add(ep, d)
	}

	for w.cand.len() > 0 {
		c := w.cand.pop()

		// Everything still queued is further than the worst result held, and
		// the results are full, so nothing reachable from here can improve.
		if worst, full := w.res.Worst(); full && c.Dist > worst {
			break
		}

		w.nbuf = h.neighbours(c.ID, level, w.nbuf)
		for _, nb := range w.nbuf {
			if w.seen(nb) {
				continue
			}
			w.mark(nb)

			nd := h.measure(q, nb, w)
			if worst, full := w.res.Worst(); full && nd >= worst {
				continue
			}

			w.cand.push(topk.Result{ID: nb, Dist: nd})
			if nb != skip {
				w.res.Add(nb, nd)
			}
		}
	}

	return w.res.Results()
}

func (h *HNSW) measure(q []float32, id int32, w *workspace) float32 {
	w.dist.inc()
	return distance.L2Squared(q, h.vec(id))
}

func (h *HNSW) Search(q []float32, k int) ([]topk.Result, error) {
	if len(q) != h.dim {
		return nil, fmt.Errorf("query has dimension %d, index has %d", len(q), h.dim)
	}
	if k < 1 {
		return nil, fmt.Errorf("k must be at least 1, got %d", k)
	}

	ef := h.EfSearch()
	if ef < k {
		ef = k
	}

	w := h.pool.Get().(*workspace)
	w.dist.clear()

	ep := h.entry
	for lc := h.maxLevel; lc > 0; lc-- {
		ep = h.descend(q, ep, lc, w)
	}
	res := h.searchLayer(q, ep, ef, 0, -1, w)

	h.distTotal.Add(w.dist.total())
	h.pool.Put(w)

	if len(res) > k {
		res = res[:k]
	}
	return res, nil
}

// Reachable counts the nodes a walk can get to from the entry point through the
// bottom layer.
//
// This was the right check with one flat graph, where every search started at
// the entry point. With layers a query enters level 0 wherever the descent
// dropped it, which differs per query, so a low number here no longer means
// those nodes are unfindable. Use Components for that.
func (h *HNSW) Reachable() int {
	seen := make([]bool, h.Len())
	return h.walk(h.entry, seen)
}

// Components walks the level-0 graph from each node not yet seen and returns
// how many walks it took to cover everything, plus the largest set one walk
// reached.
//
// Edges are added in both directions but pruning can drop one side, so the
// graph is directed and these are reachable sets rather than true undirected
// components. A count of 1 still says what matters: every node is reachable
// from the first one. A higher count means the graph came apart somewhere, and
// a query whose descent lands in one piece can't reach vectors in another
// however high efSearch goes.
func (h *HNSW) Components() (count, largest int) {
	seen := make([]bool, h.Len())
	for id := range h.links {
		if seen[id] {
			continue
		}
		size := h.walk(int32(id), seen)
		count++
		if size > largest {
			largest = size
		}
	}
	return count, largest
}

func (h *HNSW) walk(from int32, seen []bool) int {
	if seen[from] {
		return 0
	}
	seen[from] = true

	queue := []int32{from}
	count := 1

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, nb := range h.links[node][0] {
			if !seen[nb] {
				seen[nb] = true
				count++
				queue = append(queue, nb)
			}
		}
	}
	return count
}

// Degrees reports the bottom layer's neighbour counts. A max above mMax0 means
// the prune in link is not firing.
func (h *HNSW) Degrees() (min, max int, mean float64) {
	min = -1
	total := 0
	for _, per := range h.links {
		d := len(per[0])
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

// LevelSizes returns how many nodes live at each level, bottom first. Should
// shrink by roughly a factor of M each step up.
func (h *HNSW) LevelSizes() []int {
	sizes := make([]int, h.maxLevel+1)
	for id := range h.links {
		for lc := 0; lc <= h.levelOf(int32(id)); lc++ {
			sizes[lc]++
		}
	}
	return sizes
}

var _ Index = (*HNSW)(nil)
