# Decisions

## Go

Python is too slow for a benchmark against FAISS. Rust is better on paper, but
HNSW is a graph of mutually pointing nodes, the worst case for the borrow
checker. Go has bench, pprof and race in the toolchain.

Cost: no SIMD. AVX2 in assembly later, measured not assumed.

## Flat storage, not [][]float32

One slice, `At(i)` returns a window into it. One allocation instead of a million.

`At` returns `Data[lo:hi:hi]`. Without the third number an append runs into the
next vector.

## No square root in L2

Monotonic, so the ranking is the same. One less op per comparison.

## Bounded heap, not sort

O(n log k) against O(n log n), roughly 6x fewer comparisons at n=1M, k=10.
`Add` is 1.96 ns, most candidates lose on the first compare.

## Ties broken by ID

Equal distances are common. Without a rule, serial and parallel runs disagree
and recall stops being comparable.

## Benchmarks

Used `-benchtime=200x` at first and got 53 ns for `topk.Add`. Real answer is
1.96 ns. Default benchtime, three runs, take the median.

Flat, 100k x 128, single thread: 8.98 / 8.41 / 8.28 ms. About 84 ms at 1M,
roughly 12 QPS. That's the number to beat.

Each query streams 51 MB past a 24 MB L3, so it's memory bound. Caps what AVX2
can do.
