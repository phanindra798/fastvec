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
workers in order, so lowest-ID already wins.

Keeping it. HNSW visits nodes in graph order, so there it's the only thing
making a build reproducible.

## Recall by distance, not by ID

SIFT1M gave 0.9994 for exact search, which shouldn't be possible. 56 queries
disagreed, every one an exact tie, and the ID gaps repeated: 78816 and 64278.
The dataset has duplicate vectors. Ground truth picked one copy, we pick the
lower ID, both are nearest.

Recall now counts a result correct if it's at least as close as the k'th listed
neighbour. Same as ann-benchmarks. Diagnostic behind `FASTVEC_DIAG=1`.

## Benchmarks

`-benchtime=200x` gave 53 ns for `topk.Add`. Real answer 1.96 ns. Default
benchtime, three runs, median.

Flat scan, 100k x 128:

    workers   1      2      4      6      8      12     16
    ms      7.97   3.94   2.44   1.95   2.21   1.92   1.75

8 workers is slower than 6. Six P-cores, four E-cores, and the merge waits for
the slowest. 4.56x from 16 threads, not 16x: each query moves 51 MB in 1.75 ms,
about 29 GB/s, near what the memory can deliver. Caps what AVX2 can do.

SIFT1M exact pass: 10,000 queries, 2m45s, 16.5 ms each, 60 QPS on 16 threads,
12 QPS on one. Recall 1.0000.

## What hnswlib does on the same data

Measured before writing any HNSW, so there's a target instead of a guess.
M=16, efConstruction=200, single thread, SIFT1M.

    ef        10      40      60     100     200     500
    recall  0.708   0.928   0.960   0.984   0.996   0.9997
    QPS     19907    7650    5286    3771    1999      864

Build 92.6s for a million vectors.

5286 QPS at 96% recall against our 12. Roughly 440x for 4% of the answers being
different. That gap is why approximate search exists.

Aiming to land within a few times of that curve. Beating it isn't the goal.

## Pruning at MMax, not M

Stage 1 first came out at recall 0.61 and failed its gate. Reachability said
why: 23,214 of 100,000 nodes had no path from the entry point.

Degree stats gave the cause away, min=16 max=16 mean=16.0. Every list
permanently full, so every new link evicted an old one, and across 1.6M link
operations plenty of those were some node's last inbound edge. The node keeps
its outbound links, which is why nothing looks wrong until you check who points
at it.

Two parameters collapsed into one. M is how many neighbours you pick for a new
node; MMax is the cap at which an existing list gets pruned. Using M for both
prunes far harder than intended.

    prune at        M          2M
    orphans     23,214          4
    recall       0.6084     0.9863

Everything else unchanged. Build 48.6s for 100k, 2206 QPS at ef=100 against
Flat's 92.

The 0.70 gate was too low. A well connected flat graph over 100k with ef=100 is
easier than I assumed. SIFT1M at stage 4 is the real test.

## Stage 1 sweep

    ef        10      20      50     100     200
    recall  0.682   0.837   0.950   0.986   0.996
    QPS     10248    6755    3744    2340    1393

The dial behaves. Recall climbs, throughput falls.

## SIFT-100k for the gates

First 100k base vectors and 1000 queries from SIFT1M, ground truth computed by
Flat in 2.1s. The full million takes minutes per check, too slow to iterate on.

1000 queries rather than the 100 siftsmall ships. Recall over 100 queries
carries about +/-3 points of noise and the gates sit at 0.90, which would make
them coin flips.
