# Decisions

## Go

Python is too slow for a benchmark against FAISS. Rust is better on paper but
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

Deleted the rule and ran everything. `topk` fails at once; the index tests
don't, because Flat scans in ID order and SearchN merges workers in order, so
lowest-ID already wins. I'd written down that serial and parallel would
disagree without it. Wrong.

Kept anyway. HNSW visits nodes in graph order.

## Recall by distance, not by ID

SIFT1M gave 0.9994 for exact search, which shouldn't be possible. 56 queries
disagreed, every one an exact tie, ID gaps repeating at 78816 and 64278. The
dataset has duplicate vectors: ground truth picked one copy, we pick the lower
ID, both are nearest.

Recall now counts a result correct if it's at least as close as the k'th listed
neighbour. Same as ann-benchmarks. Diagnostic behind `FASTVEC_DIAG=1`.

## Benchmarks need enough iterations

`-benchtime=200x` gave 53 ns for `topk.Add`. Real answer 1.96 ns. Default
benchtime, three runs, median.

## Baselines

Flat, 100k x 128, one thread: 8.98 / 8.41 / 8.28 ms.

    workers   1      2      4      6      8      12     16
    ms      7.97   3.94   2.44   1.95   2.21   1.92   1.75

8 workers beats 12 and loses to 6. Six P-cores, four E-cores, and the merge
waits for the slowest. 4.56x from 16 threads: each query moves 51 MB in 1.75 ms,
about 29 GB/s, near what the memory delivers. Caps what AVX2 can do.

SIFT1M exact: 10,000 queries, 2m45s, 16.5 ms each, 60 QPS on 16 threads, 12 on
one. Recall 1.0000.

hnswlib on the same machine, M=16, efConstruction=200, one thread, SIFT1M:

    ef        10      40      60     100     200     500
    recall  0.708   0.928   0.960   0.984   0.996   0.9997
    QPS     19907    7650    5286    3771    1999      864

Build 92.6s. 5286 QPS at 96% recall against our 12, so roughly 440x for 4% of
the answers differing.

## SIFT-100k for the gates

First 100k base vectors and 1000 queries from SIFT1M, ground truth computed by
Flat in 2.1s. 1000 queries not the 100 siftsmall ships: recall over 100 carries
about +/-3 points of noise and the gates sit at 0.90.

## Pruning at MMax, not M

Stage 1 first came out at recall 0.61. Reachability said why: 23,214 of 100,000
nodes had no path from the entry point.

Degree stats gave the cause away, min=16 max=16 mean=16.0. Every list
permanently full, so every new link evicted an old one, and across 1.6M link
operations plenty of those were some node's last inbound edge.

M is how many neighbours you pick for a new node; MMax is the cap at which an
existing list gets pruned. Using M for both prunes far harder than intended.

    prune at        M          2M
    orphans     23,214          4
    recall       0.6084     0.9863

## Stage 1 sweep

    ef        10      20      50     100     200
    recall  0.682   0.837   0.950   0.986   0.996
    QPS     10248    6755    3744    2340    1393

## Distance counter behind a build tag

Wall clock can't separate a smarter graph from a quieter machine. Counting
distance computations can, so there are two files, `//go:build diagnostic` and
`//go:build !diagnostic`. Normal builds compile the counter away.

Never publish a timing number from a diagnostic build.

## Stage 2, layers

Kept `Params.SingleLayer` so the flat version can still be built, otherwise the
comparison is me remembering yesterday's numbers.

Level sizes [100000 6201 397 17 2], shrinking by roughly M each step, as the
geometric draw predicts.

    ef=20    single 0.8369, 550 dist    layered 0.8550, 481 dist
    ef=50    single 0.9500, 909 dist    layered 0.9470, 831 dist
    ef=100   single 0.9863, 1407 dist   layered 0.9756, 1327 dist

6-13% fewer nodes touched.

## Layers fragmented the graph

    single layer   5 components, largest 99,996 of 100,000   ( 0.0% stranded)
    layered        6 components, largest 54,217 of 100,000   (45.8% stranded)

Same data, same seed, only the hierarchy differs.

Stage 1 inserted by searching the whole graph from node 0, so links pointed back
toward a common region. Stage 2 descends to a local entry point and searches
from there, so links stay local and clusters never join.

Recall survives at 0.9756 because components are spatially coherent, the descent
drops a query into the region its neighbours live in. Queries near a boundary
lose the neighbours across it, which is the 1% gap against flat.

Nearest-M selection causes it. Stage 3's heuristic keeps candidates pointing
where no closer neighbour does, which is the missing edge between clusters.
Logged not failed, so before and after can be compared.

## A check that went stale

`Reachable` counted nodes reachable from the entry point through level 0.
Correct for one flat graph, where every search started there.

With layers it reported 45.8% unreachable while recall was 0.9756. Both can't be
true. My first guess was the metric had gone stale and the graph was fine.
Wrong, `Components` showed six real pieces. Misleading metric and real
fragmentation at once.
