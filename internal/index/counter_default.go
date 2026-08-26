//go:build !diagnostic

package index

// CountingEnabled is false in a normal build. All the counter methods compile
// away, so timing numbers aren't paying for instrumentation.
const CountingEnabled = false

type distCounter struct{}

func (distCounter) inc()          {}
func (distCounter) total() uint64 { return 0 }
func (distCounter) clear()        {}
