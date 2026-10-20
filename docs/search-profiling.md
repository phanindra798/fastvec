# Search profiling and allocation improvement

Measured on the same Windows/i7-12650H environment as the repeated SIFT1M
benchmarks, using Go 1.26.5, one query thread, the existing SIFT1M graph, and
10,000 SIFT queries. k=10, ef=100. The working checkout was uncommitted.

## Findings

A 10-second Go benchmark with CPU profiling collected 23.31 seconds of CPU
samples across setup, warmups, calibration and measurement. Approximate flat
sample shares were:

| Function | CPU sample share |
|---|---:|
| AVX2 squared-distance kernel | 60.1% |
| Graph neighbour access | 12.5% |
| Visited-node lookup | 9.7% |
| Result heap sift-down | 3.1% |

These are sampled attribution, not isolated kernel timings. Cache and memory
stalls may be charged to the instruction accessing data. They do not prove
that arithmetic alone is the bottleneck. Setup appears in CPU and memory
profiles even though benchmark ns/op and B/op exclude setup and warmup.
The allocation profile is dominated by loading the graph; query allocations
are concentrated in `topk.Collector.Results`.

## Change and validation

Replaced reflection-based `sort.Slice` with typed `slices.SortFunc`, retaining
the same distance-first, ID-second ordering and independent result copy.
Existing brute-force comparisons exercise ties, shuffled inputs, empty inputs,
and retained result slices. Synthetic recall and concurrent query tests pass.

Three before/after pairs used separate compiled binaries and the same graph,
with 10,000 measured queries per execution. Execution order was before/after,
after/before, before/after. Profiling was disabled for these timing comparisons.

| Pair | Before ns/query | After ns/query |
|---|---:|---:|
| 1 | 256,450 | 206,741 |
| 2 | 211,960 | 211,958 |
| 3 | 208,352 | 223,489 |
| Median | 211,960 | 211,958 |

Every before run reported 952 B/query and 3 allocations/query; every after run
reported 896 B/query and 1 allocation/query. This removes two allocations and
56 bytes per query at ef=100. It does **not** demonstrate a throughput gain:
the median times are essentially equal, and observations vary. The remaining
allocation is the caller-owned result slice. The published comparison with
hnswlib predates this change and has not been relabelled as a new measurement.

The faster tiny-input path exposed the benchmark calibration ceiling: a million
sweeps can take less than 100 ms. Both Go and Python now allow up to 2^24 sweeps,
retaining the same 100 ms target and finite failure limit for stalled clocks.
Deterministic clock tests cover the formerly rejected fast-sweep case.

## Reproduce

From the repository root in PowerShell, using your local dataset paths:

```powershell
$env:FASTVEC_BENCH_INDEX='D:/data/sift.fvi'
$env:FASTVEC_BENCH_QUERIES='D:/data/sift/sift_query.fvecs'
go test ./internal/index -run '^$' -bench 'BenchmarkHNSWSearch/ef100$' -benchtime 10s -cpu 1 -cpuprofile search.cpu -memprofile search.mem -o search.test.exe
go tool pprof -top search.test.exe search.cpu
go tool pprof -top -alloc_space search.test.exe search.mem
go test ./internal/index -run '^$' -bench 'BenchmarkHNSWSearch/ef100$' -benchtime 10000x -count 3 -cpu 1
```

The benchmark skips unless both dataset environment variables are set. Use
`BenchmarkHNSWSearch/ef500$` for the higher-recall workload. Profiles and test
binaries are local development artifacts, not required repository files.

Next, investigate data access in distance evaluation and graph traversal. Any
layout or kernel change should retain recall and be compared in alternating
before/after runs; this profile alone is insufficient to promise a speedup.

## Graph access inlining experiment (reverted)

Tested extracting the lock-and-copy build path from `HNSW.neighbours` into a
separate method, leaving the immutable query path small enough to inline.
Go compiler diagnostics confirmed inlining into both descent and layer search.
The graph representation, neighbour order and distance calculations were unchanged.

Compared separate binaries using the same saved SIFT1M graph and all 10,000
queries at k=10, one CPU, with a full query warmup outside benchmark timing.
Three pairs ran in before/after, after/before, before/after order. Each measured
exactly 10,000 queries per ef. The baseline includes the earlier typed sorting
change. No CPU profiling ran during these comparisons.

| Pair | ef=100 before ns/query | ef=100 after | ef=500 before | ef=500 after |
|---|---:|---:|---:|---:|
| 1 | 220,016 | 219,092 | 932,301 | 956,982 |
| 2 | 209,706 | 229,263 | 930,605 | 918,174 |
| 3 | 210,872 | 217,929 | 880,521 | 877,103 |
| Median | 210,872 | 219,092 | 930,605 | 918,174 |

Lower times are better. Median time increased 3.9% at ef=100 and decreased
1.3% at ef=500, with inconsistent pairwise outcomes. All runs allocated once
per query: 896 bytes at ef=100 and 4096 bytes at ef=500. These exploratory
measurements do not establish a useful improvement, so the change was reverted.
Raw outputs are retained in `bench/results/neighbours-inlining-experiment`.

## Compact bottom-layer layout (retained)

After building or loading an index, bottom-layer neighbour IDs are copied into
one contiguous array with a per-node offset table. Search reads that array
directly. The existing graph slices share the same packed storage, preserving
edge order, diagnostics and persistence. The on-disk format is unchanged, and
building still uses the original mutable adjacency lists until it finishes.

Three alternating before/after pairs used the same SIFT1M index and all 10,000
queries, k=10, one CPU, and 10,000 measured searches per ef after a full warmup.
The baseline includes typed sorting and excludes the reverted inlining experiment.

| Pair | ef=100 before ns/query | ef=100 compact | ef=500 before | ef=500 compact |
|---|---:|---:|---:|---:|
| 1 | 205,455 | 189,999 | 856,935 | 800,932 |
| 2 | 197,946 | 187,082 | 854,782 | 795,441 |
| 3 | 196,724 | 191,220 | 879,461 | 876,430 |
| Median | 197,946 | 189,999 | 856,935 | 800,932 |

Median query time decreased 4.0% at ef=100 and 6.5% at ef=500. Every pair
favoured the compact layout, although the third ef=500 pair had only a small
gain. These are local exploratory observations, not confidence intervals or a
new comparison with hnswlib. Query allocations remain one: 896 bytes at ef=100
and 4096 bytes at ef=500.

Separate retained-heap observations after loading and garbage collection were
655,697,040 bytes / 2,066,602 objects before and 657,982,288 bytes / 1,066,636
objects after. Thus retained heap increased about 2.18 MiB (0.35%), while roughly
one million separately allocated neighbour arrays disappeared. These are Go heap
measurements, not process peak RSS. Compaction also temporarily allocates an
edge array plus offsets while the old lists remain reachable: roughly 88 MiB
of additional storage for this graph, before garbage collection. Load/build
completion therefore has an extra copy and a higher transient memory requirement.

Validation compared exact returned IDs **and float32 distances** for all 10,000
SIFT queries at both ef values against the original adjacency lookup, with no
differences. Synthetic checks cover sequential/four-worker builds, ties through
the existing suites, and byte-identical save/load roundtrips. The normal,
pure-Go and diagnostic short suites and Go vet pass. The local race detector
still requires the unavailable C toolchain; the configured Linux CI job remains
the place to run that check.

Raw timing and memory outputs are retained in `bench/results/compact-bottom-experiment`.
To repeat the full-dataset equality check with the environment variables above:

```powershell
go test ./internal/index -run '^TestCompactDatasetResults$' -count 1
```

Next, measure load-time and peak-memory impact explicitly before pursuing a
larger representation change. Consider filling packed storage directly during
loading if the temporary copy is material; do not change the file format solely
to obtain this query-time layout.

Follow-up: [loader profiling](load-profiling.md) measures two loading approaches
that were reverted because they did not improve the time/memory tradeoff.
