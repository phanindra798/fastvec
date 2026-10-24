# 16-bit visited-node stamps — 2026-10-22

Retained: visited-node stamps and their epoch are now uint16 instead of uint32.
For a million-node index, each workspace's stamp payload falls from 4,000,000 to
2,000,000 bytes (2 MB / about 1.91 MiB saved). This is the backing-array payload,
not a measured reduction in total process RSS. Each concurrent workspace owns
its own array; sync.Pool lifetime and concurrent searches determine the count.

The smaller array keeps the existing epoch-based visited checks and full clear
on wrap. The clear now occurs after 65,535 layer searches per workspace rather
than roughly four billion. Index format, graph layout and query result ownership
are unchanged. Construction also uses these workspaces.

## Search measurements

Four alternating baseline/candidate pairs, AC power, Windows Balanced plan,
Ubuntu/WSL2, i7-12650H, Go 1.26.9, pinned to guest CPU 0, GOMAXPROCS=1. Each process
loads the same saved SIFT1M graph, warms the 10,000 queries and measures 10,000
searches per ef. Loading and warmup are excluded. Both binaries include the same
measurement/test harness; production code differs only in stamp/epoch width.

| ef | uint32 median µs/query [min–max] | uint16 median µs/query [min–max] | Median time change |
|---:|---:|---:|---:|
| 100 | 238.9 [234.1–254.6] | 229.2 [222.6–231.5] | -4.0% |
| 500 | 979.3 [974.6–984.1] | 976.1 [934.8–1,005.2] | -0.3% |

All four ef=100 pairs favoured the candidate (1.3–9.1% lower time).
ef=500 was mixed, with two wins and two losses; no meaningful ef=500 speedup is
established. These are local exploratory observations, not confidence intervals
or a universal speed claim. Host load and thermal state were not controlled;
virtual CPU affinity does not guarantee a particular physical core.
The memory reduction is the main reason to retain the change, supported by the
observed ef=100 result and no clear ef=500 regression.

Result allocations remain one per query: 896 bytes at ef=100 and 4096 at ef=500.
The short measured search sweeps do not span a complete 16-bit epoch cycle;
separate reset benchmarks below measure that additional cost.

## Rollover cost

Three one-second repetitions on a million-slot workspace, pinned to the same
virtual CPU. The amortized case repeatedly begins a layer search and marks one
node; the rollover case forces the maximum epoch before every begin.

| Benchmark | uint32 median | uint16 median |
|---|---:|---:|
| Begin plus mark, amortized | 2.155 ns/op | 2.754 ns/op |
| Forced full-array clear | 90.620 µs/op | 40.038 µs/op |

The isolated amortized increase is about 0.6 ns per begin/mark operation.
The uint16 clear is cheaper individually but occurs much more frequently.
The forced-clear benchmark repeatedly touches the same array; it is not a
production tail-latency guarantee. A query that triggers wrap has an extra O(N)
clear, and this tradeoff may matter more for larger indexes. No allocations
occurred in either reset benchmark. These results do not imply every real
rollover takes exactly 40 µs.

## Correctness and validation

- Exact SHA-256 fingerprints of ordered result IDs and float32 distance bits
  matched for all 10,000 SIFT queries at ef=100 and ef=500 (200,000 results).
- Unit tests cover query isolation, stale epoch-one marks after wrap, clearing
  retained slots when a smaller node count is used, growth, and identical layer
  search results when rollover is forced. Tests own their workspaces and do not
  depend on sync.Pool retaining a particular entry.
- Default, purego and diagnostic short suites passed; go vet and the complete
  short race suite passed. Rollover tests also passed 20 race-enabled repeats.
- Full uncached dataset suite passed with a 60-minute timeout; the index package completed in 331.176 seconds, including large graph construction and SIFT accuracy checks.

The timed baseline source snapshot comes from commit
`f7d70d0f38353fbbe321d0a3efd54ab668aabdb5`. Binary and source hashes, commands,
power snapshots and all measurements are in the
[session](../bench/results/stamp-width-20261022/session.json).
The final code adds explanatory comments; the test harness later received import
formatting and an equivalent unsigned-decrement expression for forcing wrap.
The measured production source snapshots are retained alongside the results.

[Search summary](../bench/results/stamp-width-20261022/summary.json),
[rollover summary](../bench/results/stamp-width-20261022/rollover-summary.json)
and all raw logs are retained. Dataset/index hashes are recorded in the earlier
[baseline manifest](../bench/results/ubuntu-baseline-20261021/environment.json).

## Reproduce

Build separate baseline/candidate test binaries with the same test harness.
Use the recorded commands in session.json, with new output paths. For each:

```sh
export FASTVEC_BENCH_INDEX=/mnt/d/data/sift.fvi
export FASTVEC_BENCH_QUERIES=/mnt/d/data/sift/sift_query.fvecs
export GOMAXPROCS=1
go test -c ./internal/index -o bin/stamps16.test
bin/stamps16.test -test.run=TestSearchDatasetFingerprint -test.v=true
taskset -c 0 bin/stamps16.test -test.run='^$' -test.bench=BenchmarkHNSWSearch -test.benchtime=10000x -test.count=1 -test.cpu=1
taskset -c 0 bin/stamps16.test -test.run='^$' -test.bench=BenchmarkWorkspaceEpoch -test.benchtime=1s -test.count=3 -test.cpu=1
```

Alternate baseline/candidate, candidate/baseline, baseline/candidate,
candidate/baseline. Keep profiling separate from comparisons. The next performance
investigation remains vector access in distance evaluation, which accounts for
roughly 63–64% of samples in the preceding profiles; a smaller stamp array does
not remove that dominant cost.
