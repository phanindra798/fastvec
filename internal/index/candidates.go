package index

import "github.com/phanindra798/fastvec/internal/topk"

// candidates is a min-heap of nodes still to be examined during a search. The
// nearest one is always at the root, which is the one to expand next.
//
// topk.Collector is a max-heap capped at k, so it can't be reused here: this
// one is unbounded and pops from the other end. Same ordering rule though,
// topk.Before, so a tie resolves the same way in both.
type candidates struct {
	items []topk.Result
}

func (c *candidates) reset()   { c.items = c.items[:0] }
func (c *candidates) len() int { return len(c.items) }

func (c *candidates) push(r topk.Result) {
	c.items = append(c.items, r)

	i := len(c.items) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if !topk.Before(c.items[i], c.items[parent]) {
			break
		}
		c.items[parent], c.items[i] = c.items[i], c.items[parent]
		i = parent
	}
}

func (c *candidates) pop() topk.Result {
	top := c.items[0]

	last := len(c.items) - 1
	c.items[0] = c.items[last]
	c.items = c.items[:last]

	i, n := 0, len(c.items)
	for {
		best := i
		if l := 2*i + 1; l < n && topk.Before(c.items[l], c.items[best]) {
			best = l
		}
		if r := 2*i + 2; r < n && topk.Before(c.items[r], c.items[best]) {
			best = r
		}
		if best == i {
			break
		}
		c.items[i], c.items[best] = c.items[best], c.items[i]
		i = best
	}
	return top
}
