// Package topk keeps the k nearest results seen during a scan.
package topk

import "sort"

type Result struct {
	ID   int32
	Dist float32
}

// better reports whether a should rank above b. Equal distances fall back to
// the ID so the ordering can't depend on visit order.
func better(a, b Result) bool {
	if a.Dist != b.Dist {
		return a.Dist < b.Dist
	}
	return a.ID < b.ID
}

// Collector holds the k best results out of everything offered to it.
//
// Max heap, so items[0] is the current worst of the k: the one to compare
// against and the one to evict, both O(1) to reach.
//
// TODO: for k of 2 or 3 a linear scan is probably cheaper than the heap
// bookkeeping. Measure before HNSW starts calling this in a tight loop.
type Collector struct {
	k     int
	items []Result
}

func New(k int) *Collector {
	if k < 1 {
		k = 1
	}
	return &Collector{k: k, items: make([]Result, 0, k)}
}

// Reset empties the collector but keeps the backing array, so a search loop can
// reuse one across many queries without allocating.
func (c *Collector) Reset() {
	c.items = c.items[:0]
}

func (c *Collector) Len() int { return len(c.items) }

// Worst returns the current k'th best distance and whether the collector is
// full. A caller can use it to skip candidates that can't make the cut.
func (c *Collector) Worst() (float32, bool) {
	if len(c.items) < c.k {
		return 0, false
	}
	return c.items[0].Dist, true
}

func (c *Collector) Add(id int32, dist float32) {
	cand := Result{ID: id, Dist: dist}

	if len(c.items) < c.k {
		c.items = append(c.items, cand)
		c.up(len(c.items) - 1)
		return
	}

	if better(cand, c.items[0]) {
		c.items[0] = cand
		c.down(0)
	}
}

// Results returns the kept items nearest first. Copies, so the caller can hold
// the slice after a Reset.
func (c *Collector) Results() []Result {
	out := make([]Result, len(c.items))
	copy(out, c.items)
	sort.Slice(out, func(i, j int) bool { return better(out[i], out[j]) })
	return out
}

// up and down maintain the heap: every node ranks better than its children, so
// the root is the worst.

func (c *Collector) up(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !better(c.items[parent], c.items[i]) {
			break
		}
		c.items[parent], c.items[i] = c.items[i], c.items[parent]
		i = parent
	}
}

func (c *Collector) down(i int) {
	n := len(c.items)
	for {
		worst := i
		if l := 2*i + 1; l < n && better(c.items[worst], c.items[l]) {
			worst = l
		}
		if r := 2*i + 2; r < n && better(c.items[worst], c.items[r]) {
			worst = r
		}
		if worst == i {
			return
		}
		c.items[i], c.items[worst] = c.items[worst], c.items[i]
		i = worst
	}
}
