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

Deleted the rule and ran everything to see if it mattered. `topk` fails at once.
The index tests don't, because `Flat` scans in ID order and `SearchN` merges
workers in order, so lowest-ID already wins. I'd written down that serial and
parallel would disagree without it. Wrong.

Keeping it. HNSW visits nodes in graph order, not ID order, so there it's the
only thing making a build reproducible.

## Recall by distance, not by ID

SIFT1M gave recall 0.9994 for exact search, which shouldn't be possible. 56
queries disagreed. Recomputed in float64: every one was an exact tie, and the ID
gaps repeated, 78816 and 64278. The dataset has duplicate vectors. Ground truth
picked one copy, we pick the lower ID, both are nearest.

Recall now counts a result correct if it's at least as close as the k'th listed
neighbour. Same as ann-benchmarks. Diagnostic kept behind `FASTVEC_DIAG=1`.

## Benchmarks

`-benchtime=200x` gave 53 ns for `topk.Add`. Real answer 1.96 ns. Default
benchtime, three runs, median.

Flat, 100k x 128, one thread: 8.98 / 8.41 / 8.28 ms.

    workers   1      2      4      6      8      12     16
    ms      7.97   3.94   2.44   1.95   2.21   1.92   1.75
    speedup 1.00   2.02   3.27   4.09   3.61   4.15   4.56

8 workers is slower than 6. Six P-cores, four E-cores, and the merge waits for
the slowest.

4.56x from 16 threads. Each query moves 51 MB in 1.75 ms, about 29 GB/s, near
what the memory can deliver. Caps what AVX2 can do.

SIFT1M full pass: 10,000 queries, 2m45s, 16.5 ms each, 60 QPS, recall 1.0000.
That's the number to beat.
