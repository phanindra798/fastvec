# Decisions

Why I picked things. Writing it down as I go, otherwise I won't remember the
reasoning later.

## Go for the engine

The output of this project is a benchmark, so the language had to be fast
enough for the comparison to mean anything, and had to make measuring easy.

Python is out. Tight numeric loops run orders of magnitude slower, there's no
control over memory layout, and the GIL blocks parallel search. A brute force
scan or a graph traversal written in Python loses to FAISS by so much that the
chart says nothing.

Between Go, Rust and C++:

Go's toolchain already contains everything this project needs to prove its
claims. `go test -bench` for micro-benchmarks, `pprof` for profiling, and
`-race` built into the test runner. Since search will be concurrent and the
whole point is measured numbers, having those in the standard toolchain rather
than bolted on matters more here than it would in most projects.

Goroutines and `sync.Pool` also map cleanly onto the concurrency shape I need:
many parallel read-only searches, each holding a small private scratch buffer.

Rust would give me no GC and better SIMD. The trade is that HNSW is a graph of
mutually referencing nodes, which is exactly the shape that fights the borrow
checker hardest, and it would have meant `unsafe` or arena indices in the core
data structure on day one.

C++ has the strongest SIMD story of the three, but slower iteration, no
built-in test or benchmark runner, and manual lifetime management for the same
graph structure.

## The cost of choosing Go

No clean SIMD. Go can't emit vector instructions from normal code, and the
distance function is the hottest path in the program.

Plan is to write the L2 kernel in Go assembly using AVX2 and measure what it
actually buys, rather than assuming the theoretical 8x. The gap between the
theoretical number and the measured one is worth knowing.

GC pauses are the other cost. Mitigation is to allocate nothing per query, which
means reusing scratch buffers instead of making new ones. Whether that's enough
shows up in the p99 numbers.

## Squared distance instead of real Euclidean

TODO, write this up when I implement it.

## Bounded heap instead of sorting candidates

TODO
