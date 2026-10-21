# Correctness and security review — 2026-10-20–21

This review covers the local Go library, file readers, command-line tools,
Python benchmark helpers and CI configuration. Fastvec currently has no
network server or authentication layer.

## Corrections

- Final race run exposed a test-only nil dereference: a workspace-allocation
  test assumed `sync.Pool.Get` would reuse the last workspace. It now owns
  its workspace explicitly, retaining the same allocation-bound assertion.
- Final persistence check: reject negative or greater-than-uint32 tuning
  parameters before writing index data. Previously these values could wrap
  silently during serialization. Regression tests cover all five persisted
  tuning fields and verify rejection leaves the output untouched.
- Preserve reverse edges already added by another worker when initializing a
  node's own neighbour list during parallel construction. A quality check
  initially failed with 1.21% additional stranded nodes; inspection found the
  overwrite path. A direct regression test checks edge preservation and
  deduplication, and the existing quality test passed ten consecutive runs
  after the correction without changing its threshold.
- Check equal vector lengths before dispatching squared distance to assembly.
  Invalid internal callers now panic in Go rather than allowing assembly to
  read beyond the shorter slice.
- Bound HNSW build workers by dataset size. An excessive worker request on a
  small dataset no longer starts an excessive number of goroutines.
- Bound each parallel exact-search collector by its chunk size. Previously,
  large `k` allocated a whole-index-sized heap for every worker.
- Validate Python vecs record boundaries, dimensions and finite vector values.
  Validate dataset relationships, ground-truth IDs and supported query counts
  before calling hnswlib. Negative ground-truth IDs previously indexed from
  the end of NumPy arrays and could silently distort recall.
- Restrict the CI GitHub token to `contents: read`.
- Add a CI job for Go reachable-vulnerability checks and the pinned Python
  dependency audit, so subsequent pushes repeat these checks.
- Raise the Go patch requirement and CI version from 1.26.5 to 1.26.9.
  The original vulnerability scan reported no reachable vulnerable symbols,
  but listed patched standard-library advisories, including
  [GO-2026-6604](https://pkg.go.dev/vuln/GO-2026-6604) in an imported package.
  Historical benchmark metadata is preserved.

## Dependency provenance

Go 1.26.9 was downloaded from the official Go downloads service, with the
Windows amd64 archive checked against its published SHA-256:
`d722201a9c0c086d1610e111c48203009af690892ed708072bd5ae20160e7a59`.
Audit tools and downloaded toolchains live outside the repository.

`pip-audit 2.10.1 -r py/requirements.txt --no-deps --disable-pip`
reported no known vulnerabilities for the exact listed pins. This scan covers
the pinned package list, not arbitrary additional packages in a user's Python
environment or the native dependencies of a conda package.

The follow-up `govulncheck 1.9.0 -show verbose ./...` scan using Go 1.26.9
reported no vulnerabilities, including at package and module level.

A source scan for common private-key and access-token patterns found no
matches. Pattern scanning cannot establish the absence of every possible
secret.

## Validation

- Go 1.26.9 Windows default, purego and diagnostic modes: fresh short tests,
  vet and builds passed.
- Windows short race-detector suite passed with LLVM-MinGW and CGO enabled.
- Full Windows suite with `FASTVEC_DATA=D:/data` passed using
  `go test -count=1 -timeout=60m ./...`, including the non-short SIFT tests
  (index package: 359.587 seconds).
- Linux amd64 cross-compilation passed in default and purego modes; this
  checks compilation, not Linux runtime behavior.
- All 18 Python tests passed with the pinned NumPy installation.
- The validated Python baseline completed a SIFT-small run.
- Saved benchmark summaries still match their raw runs; JSON and local
  documentation link checks passed.

The smoke run is a correctness check, not a replacement performance
measurement. These changes do not revise the historical throughput claims.

The final save-format guard was checked with fresh default, purego and
diagnostic short suites, vet and builds, plus the complete short race suite.
The affected allocation/persistence tests passed 30 race-enabled repetitions.
All 18 Python tests, artifact checks and `git fsck --no-reflogs` also passed.
The full dataset run listed above predates this final persistence-only change.

## Trust boundary and remaining limits

Use trusted local dataset and index files. These parsers load data into memory;
they do not provide a configurable total-memory budget or an isolation boundary
for hostile uploads. A large or crafted file can still exhaust process memory.
The index format validates graph structure but is not authenticated.

The caller owns input vector storage used by constructed indexes and must not
mutate it concurrently with construction or search. No service-level request
limits, authorization, encrypted transport or tenant isolation are implemented,
because there is currently no service.

GitHub CI has now passed the Linux and Windows test matrices, Linux race
detection and Python tests. A successful review and dependency scan do not
establish that all bugs have been eliminated.

## GitHub CI follow-up — 2026-10-21

[Run 39594983730](https://github.com/phanindra798/fastvec/actions/runs/39594983730)
passed eight of nine jobs. The remaining Python audit reported eight
vulnerabilities in Pillow 12.3.0, all listing 13.0.0 as the fixed version.
This supersedes the earlier clean audit result for that old pin. The dependency
is now pinned to Pillow 13.0.0, and a fresh local audit of the updated requirements
reports no known vulnerabilities. The updated commit still needs a new CI run.

The Go audit passed in that run with no reachable vulnerabilities; it reported
one module-level advisory in unused code. Audit results reflect the database
and platform at the time of each run, not a permanent security guarantee.
