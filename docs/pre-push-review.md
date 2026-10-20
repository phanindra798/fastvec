# Pre-push review — 2026-10-20

Reviewed the accumulated index/loader changes, benchmark tools, tests, CI
configuration, recorded results and documentation. This is a local review, not
a claim that all possible bugs or all supported environments have been tested.

## Issues found and fixed

- Oversized HNSW search/construction widths and exact-search result counts
  could allocate scratch storage far larger than the index. Heap capacity is
  now bounded by node count. Regression tests cover huge requested counts,
  parallel exact search and a persisted oversized search width.
- Repeated-benchmark summaries could divide by zero for malformed query counts
  and accept invalid integer metadata. Positive integer counts are now checked
  before arithmetic; tests include zero, negative, boolean and fractional values.
- The hnswlib baseline guessed version 0.8.0 when the module had no version
  attribute. It now checks installed distribution metadata and reports unknown
  when neither source is available. Historical measurements retain their
  independently verified package provenance and have not been rewritten.

## Validation completed

- Fresh short Go tests, vet and builds in default, purego and diagnostic modes
  on Windows/amd64, Go 1.26.5.
- All 15 Python standard-library tests.
- All 10,000 SIFT1M queries return identical IDs and float32 distances through
  compact and original adjacency lookup paths at ef=100 and ef=500.
- Exact search matches SIFT-small ground truth with recall@10=1.0000.
- SIFT1M and SIFT-small repeated summaries exactly match all five raw pairs.
- Benchmark JSON parses without non-finite JSON constants; local Markdown
  links resolve; Git whitespace checks pass.
- Full Windows Go suite with `FASTVEC_DATA=D:/data`,
  `go test -count=1 -timeout=60m ./...`: passed, including the non-short
  dataset tests (index package: 323 seconds).
- Windows race detector: `go test -short -race -count=1 -timeout=15m ./...`
  passed with CGO enabled and Clang 23.1.3.
- All pinned Python packages resolved on PyPI. The eleven Python/wheel
  dependencies were installed at the exact `py/requirements.txt` versions
  into a separate validation directory. hnswlib 0.8.0 used the previously
  verified conda-forge Python 3.12 Windows binary. All 15 Python tests,
  SIFT-small baseline execution and repeated-session chart generation passed
  with this environment. This does not establish that a Windows source build
  of hnswlib via `pip install -r py/requirements.txt` works without MSVC.

### Compiler provenance and local setup

The portable compiler came from the upstream
[LLVM-MinGW release](https://github.com/mstorsjo/llvm-mingw/releases/tag/20261006):
`llvm-mingw-20261006-ucrt-x86_64.zip`. Its SHA-256 matched the release asset
digest:
`317492c456aa27ee607a5919f1d2d38dcdc1112516a24d0bf4b00d078f52d17a`.
The compiler, Python packages and smoke-test outputs are outside the Git
repository.

For the race run, the compiler's `bin` directory was added to the process
PATH, `CC=clang` and `CGO_ENABLED=1` were set, and `TEMP`, `TMP` and
`GOTMPDIR` pointed at a writable workspace temporary directory. The first
attempt failed because the linker could not write into the sandbox's default
temporary folder; the rerun with these paths passed. These environment
settings were process-local.

The saved throughput measurements predate these review fixes. No new speed
claim is made from this validation run.

## Checks still pending

Linux execution still requires the configured GitHub CI jobs. Local WSL
enumeration returns access denied, and Docker's Linux engine is not running.
Windows race detection is now verified; Linux execution is not. The project
is ready to push for CI validation; a release should wait for all required
jobs to pass. Optional diagnostic/profiling tests that require their own
environment flags are not implied by the full-suite result above.
