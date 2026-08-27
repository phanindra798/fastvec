# Decisions

Why things are the way they are. Numbers live in benchmarks.md.

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

O(n log k) against O(n log n), roughly 6x fewer comparisons at n=1M, k=10. Most
candidates lose on the first compare and never touch the heap.

## Ties broken by ID

Deleted the rule and ran everything. `topk` fails at once; the index tests
don't, because Flat scans in ID order and SearchN merges workers in order, so
lowest-ID already wins. I'd written down that serial and parallel would disagree
without it. Wrong.

Kept anyway. HNSW visits nodes in graph order, not ID order.

## Recall by distance, not by ID

Exact search scored 0.9994 on SIFT1M, which shouldn't be possible. Every
disagreement was an exact tie, and the ID gaps repeated. The dataset has
duplicate vectors: ground truth picked one copy, we pick the lower ID, both are
nearest.

Recall now counts a result correct if it's at least as close as the k'th listed
neighbour. Same as ann-benchmarks. Diagnostic behind `FASTVEC_DIAG=1`.

## Benchmarks need enough iterations

`-benchtime=200x` gave 53 ns for `topk.Add`, off by 27x. Default benchtime,
three runs, median.

## Distance counter behind a build tag

Wall clock can't separate a smarter graph from a quieter machine. Counting
distance computations can, so there are two files, `//go:build diagnostic` and
`//go:build !diagnostic`. Normal builds compile the counter away.

Never publish a timing number from a diagnostic build.

## Pruning at MMax, not M

M is how many neighbours you pick for a new node. MMax is the cap at which an
existing list gets pruned. I used M for both, which prunes so hard that every
list is permanently full and every new link evicts an old one. Across 1.6M link
operations plenty of those were some node's last inbound edge.

## Keeping the old versions buildable

`Params.SingleLayer` and `Params.NearestM` build the pre-hierarchy and
pre-heuristic versions. Without them, every claim about what layers or the
heuristic bought would be me remembering last week's numbers.

## The neighbour heuristic

Nearest-M keeps the M closest candidates, so inside a dense cluster every node
links only to others in that cluster and a walk that enters can't leave.

The paper's heuristic keeps a candidate only when it is closer to the inserting
node than to any neighbour already chosen. Anything hiding behind an existing
pick is dropped however near it is, because that direction is covered. What
survives points different ways, and those are the edges out of a cluster.

Result: sparser graph, better connected. Costs M^2 distances per selection.

## Pruning makes the graph directed

`link` adds an edge both ways, then prunes, and the prune can drop the side it
just added. So A can list B while B doesn't list A.

That's fine for search, which only ever follows outbound edges, but it means
`Components` is measuring reachable sets rather than undirected components. The
count of 1 still says the thing worth saying: everything is reachable from the
first node. Worth knowing the distinction before claiming the graph is
"connected".

## A check that went stale

`Reachable` counted nodes reachable from the entry point through level 0.
Correct for one flat graph, where every search started there.

With layers it reported 45.8% unreachable while recall was 0.9756. Both can't be
true. My first guess was that the metric had gone stale and the graph was fine.
Wrong: `Components` showed six real pieces. Misleading metric and real
fragmentation at the same time.
