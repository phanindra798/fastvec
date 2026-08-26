//go:build diagnostic

package index

// CountingEnabled is true under -tags diagnostic. Build that way to find out
// how many distances a query actually computes, which is what separates a
// smarter graph from a faster machine.
//
// Never publish a timing number from a diagnostic build.
const CountingEnabled = true

type distCounter struct{ n uint64 }

func (c *distCounter) inc()          { c.n++ }
func (c *distCounter) total() uint64 { return c.n }
func (c *distCounter) clear()        { c.n = 0 }
