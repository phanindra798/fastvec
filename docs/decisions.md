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

## AVX2 kernel, and detecting it without a dependency

Go cannot emit vector instructions from normal code, so the distance function is
about 50 lines of assembly. Two accumulators rather than one, because the
multiply and the add each have latency and a single chain stalls on itself.

Feature detection is written here rather than pulled from
golang.org/x/sys/cpu, which would have been the only third party dependency in
the project. Three things have to hold before touching YMM registers: the CPU
reports AVX2, the CPU reports AVX, and the OS has agreed to save YMM state
across a context switch. That last one is the trap, since a CPU can advertise
AVX2 while the OS has not enabled the wider state, and using it then corrupts
registers rather than faulting.

The kernel is not bit identical to scalar and does not need to be. Scalar keeps
one running sum; the kernel keeps eight partial sums and adds them at the end,
and floating point addition is not associative. Tests check a relative tolerance
and, more importantly, that the kernel never disagrees with scalar about which
of two vectors is nearer.

## Measuring the speedup honestly

First attempt compared the kernel against L2SquaredScalar and got 8.1x. Wrong
baseline: Go will not inline a function containing a loop, so calling
L2SquaredScalar directly measures a different code shape from calling L2Squared,
which is what the index actually uses. The two differ by 20%.

Matched comparison is the same public function under two build tags, purego
against the default. 81.74 to 12.71 ns, 6.4x.

## Parallel build, and why it is not the default

BuildWorkers above 1 inserts across goroutines. Each node carries a mutex over
its neighbour lists; traversal copies a list under that lock rather than reading
it live, since another worker may be appending. Entry point and max level sit
behind one more mutex.

Levels are drawn up front on a single goroutine. The RNG is stateful, so having
workers pull from it would make level assignment depend on scheduling. Drawn in
ID order it stays identical to a sequential build, and a test checks that.

The ordering that makes a node safe to follow: h.links[id] is allocated before
anything links to id, and a worker only sees id in another node's list after
taking that node's lock. The lock release orders the allocation ahead of the
read.

It costs quality. Sequentially a node is inserted into a graph where everything
before it is fully linked. In parallel many nodes are half linked at once, so a
new node searches an incomplete graph and picks worse neighbours. On 100k:

    build    32.9s to 3.79s      8.7x
    stranded 0 to 284 nodes      0.28%
    recall   0.9961 to 0.9925    ef=100, and the same third of a point at 20 and 200

Consistent across the sweep and matching the stranded fraction almost exactly,
so it is a real cost rather than noise.

Default stays sequential. It is the only mode that produces the same index
twice, and byte identical rebuilds are worth more here than build time, since
the whole project is an argument about measurement. Parallel is there for when
build time is what matters, with the cost written down.

## Pruning makes the graph directed

`link` adds an edge both ways, then prunes, and the prune can drop the side it
just added. So A can list B while B doesn't list A.

That's fine for search, which only ever follows outbound edges, but it means
`Components` is measuring reachable sets rather than undirected components. The
count of 1 still says the thing worth saying: everything is reachable from the
first node. Worth knowing the distinction before claiming the graph is
"connected".

## Index persistence

Custom binary format, little endian. Header, then how many levels each node
occupies, then the adjacency lists, then the vectors.

Vectors go in the file rather than being read back from the dataset, so a
loaded index works on its own. Costs size, SIFT1M comes to about 550 MB of
which 512 MB is the vectors, but an index that needs the original .fvecs
alongside it is half an index.

Written because re-measuring throughput meant an 11 minute rebuild every time.
The 100k index loads in 164 ms against a 33 second build, and SIFT1M in roughly
2 seconds.

Tests cover the round trip returning identical results, the same seed producing
byte-identical files, and rejection of empty, truncated, wrong-magic and
future-version files.

## A check that went stale

`Reachable` counted nodes reachable from the entry point through level 0.
Correct for one flat graph, where every search started there.

With layers it reported 45.8% unreachable while recall was 0.9756. Both can't be
true. My first guess was that the metric had gone stale and the graph was fine.
Wrong: `Components` showed six real pieces. Misleading metric and real
fragmentation at the same time.
