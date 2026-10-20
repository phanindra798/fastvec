# Loader time and peak memory

2026-10-20: native Windows, i7-12650H, Go 1.26.5, GOMAXPROCS=1,
existing SIFT1M index. Each observation starts a fresh test process and loads
the index once. Three pairs alternate before/after, after/before, before/after.
OS file-cache state is not controlled; these are not cold-disk measurements.

`TestLoadMeasurement` reports load duration and Go allocation counters.
`py/measure_load_windows.py` reads Windows PeakWorkingSetSize while the process
runs. This measures peak process resident memory, including runtime overhead,
not just the graph or retained Go heap. A short pause after loading allows the
monitor to observe the peak counter. Benchmark printing and this pause are
excluded from reported load duration.

## Experiments

| Experiment | Baseline median load | Candidate median load | Baseline median peak working set | Candidate median peak working set |
|---|---:|---:|---:|---:|
| Compact before loading vectors | 606.9 ms | 631.1 ms | 765.9 MB | 766.1 MB |
| Append bottom edges directly into packed storage | 587.4 ms | 654.4 ms | 765.9 MB | 898.0 MB |

Memory uses decimal MB. The first baseline/candidate pair in the first experiment
both reached about 803 MB; all raw observations are retained in
`bench/results/load-compaction-order.json` and `bench/results/load-direct.json`.

Moving compaction earlier made the old lists eligible for collection sooner,
but did not force collection and did not lower observed peak memory. Direct
streaming eliminated approximately one million small allocations but repeatedly
grew and copied its packed array. Total allocated bytes increased from about
750.5 MB to 1,067.9 MB, peak memory increased about 17%, and median load time
increased about 11%. Both candidates were reverted. The production loader and
the retained compact query layout remain as they were before these experiments.

## Reproduce

Build test binaries for each revision, then run:

```powershell
go test -c ./internal/index -o load.test.exe
python py/measure_load_windows.py --before before.test.exe --after after.test.exe --index D:/data/sift.fvi --out bench/results/new-load-session.json
```

The JSON output path must be new. `TestLoadMeasurement` skips unless
`FASTVEC_LOAD_MEASURE` is set; the Python runner sets it for child processes.
The Windows monitor requires no third-party Python packages. The loader test
itself is portable; only the peak-working-set monitor is Windows-specific.

Next: test a two-pass path for seekable readers. First validate/count graph
edges, then rewind and fill an exactly sized packed array. Preserve the existing
streaming path for non-seekable readers and all corruption checks. Extra parsing
and reads may offset the memory benefit, so measure it before adoption. Avoid
preallocating from untrusted header maxima without validating actual graph data.

## Two-pass seekable loader (retained)

The follow-up now validates and counts graph edges in a first pass, then rewinds
and fills an exactly sized packed bottom-layer array. The second pass validates
the data again and rejects changed bottom-layer counts before filling slices.
Buffered read-ahead and a nonzero input starting position are accounted for.
Non-seekable streams, or readers unable to report their current position, retain
the original one-pass loading and compaction path. Rewind failure returns an error.
The on-disk format and query representation are unchanged.

Three fresh-process alternating pairs on the same SIFT1M file produced:

| Pair | Before load ms | Two-pass load ms | Before peak MB | Two-pass peak MB |
|---|---:|---:|---:|---:|
| 1 | 564.1 | 554.1 | 766.0 | 708.2 |
| 2 | 576.9 | 551.7 | 765.9 | 671.2 |
| 3 | 578.1 | 562.1 | 765.9 | 671.1 |
| Median | 576.9 | 554.1 | 765.9 | 671.2 |

Median load time decreased about 4.0%; median peak working set decreased about
12.4% (94.7 MB). Total allocated bytes fell from 750.5 MB to 660.6 MB, with
approximately one million fewer allocations. Peak process memory is not the same
as retained heap, and the first candidate process had a higher peak than the
following two. These are exploratory cached-file measurements, not a guarantee
for slow disks or remote storage: the graph is read twice. Raw observations are
in `bench/results/load-twopass.json`.

Validation includes corruption tests, byte-identical roundtrips through seekable
and streaming paths, embedded indexes at nonzero offsets, unavailable current
positions, failed rewinds, and changed counts between passes. The full-dataset
query comparison and the normal/pure-Go/diagnostic short suites are also run.

Next, compare loading from uncached or slower storage before deciding whether
an explicit loader option is useful. Do not infer cold-storage performance from
these cached-file runs.

## Read-bandwidth simulation and explicit policy

An opt-in test now paces total bytes read to 100,000,000 bytes/second,
including rereads after seeking. It compares three alternating SIFT1M pairs.
This is a controlled bandwidth simulation, **not** a physical cold-disk test:
it does not model seek latency, network jitter, or the OS cache's benefit for
rereads. It represents a case where rereading bytes incurs the bandwidth cost
again. Compute time overlaps the pacing budget.

The two-pass path reads 698,718,400 bytes versus 605,346,620 bytes for single
pass, including buffered read-ahead. In this simulation, loading takes roughly
6.99 seconds versus 6.08 seconds: about 0.91 seconds saved by avoiding rereads.
Raw results from the explicit-options API are in
`bench/results/load-paced-options.txt`. These simulated times must not replace
the actual cached-file measurements above.

Callers can now choose the tradeoff explicitly:

```go
// Default: lower peak memory when seeking is available.
h, err := index.LoadHNSW(reader)

// Avoid graph rereads; accept the original temporary adjacency memory cost.
h, err = index.LoadHNSWWithOptions(reader, index.LoadOptions{SinglePass: true})
```

The zero options value has exactly the existing default policy. SinglePass
never calls Seek, even on seekable inputs. Both paths validate the format and
produce the same compact query representation. This option does not reduce
steady-state index memory or change query results. Byte-identical roundtrips,
no-seek behavior and truncated data are covered by tests. This is an internal
Go API option; the benchmark CLI also exposes it as described below.

To repeat the simulation:

```powershell
$env:FASTVEC_LOAD_SLOW='D:/data/sift.fvi'
go test ./internal/index -run '^TestLoadPacedComparison$' -v -count 1
```

The test is skipped in short mode or when the dataset environment variable is
unset.

## Benchmark command option

`fastvec-bench` accepts `-index-load-policy auto|single-pass`. The default is
`auto`, preserving two-pass loading when seeking is available. `single-pass`
avoids graph rereads when loading an existing cache. Both policies perform the
same dataset and construction-setting compatibility checks before cache reuse.
Invalid policy names are rejected before dataset I/O.

```powershell
go run ./cmd/fastvec-bench -data D:/data/sift -name sift -index D:/data/sift.fvi -index-load-policy single-pass -out bench/results/single-pass-run
```

JSON now records `index_load_policy`, the **requested** policy. It does not claim
a load occurred: the option has no effect when a missing cache must be built or
when no `-index` path is supplied. `auto` names a policy, not a guarantee that a
reader actually supports seeking. Older result files remain unchanged.

The repeated-run script forwards the policy to Fastvec, records the command,
and retains it in the summary. It rejects mixed policies within one
implementation's repetitions. hnswlib and historical JSON without this field
retain an unknown/null value; no loader policy is inferred for them.

```powershell
python py/repeat_benchmarks.py --fastvec ./fastvec-bench.exe --data D:/data/sift --name sift --index D:/data/sift.fvi --index-load-policy single-pass --out bench/results/new-single-pass-session
```

Rebuild the Fastvec executable before using the new flag. The loader measurement
tools remain necessary for load timing and peak memory: benchmark QPS measures
queries and does not include loading.
