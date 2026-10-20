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

The saved throughput measurements predate these review fixes. No new speed
claim is made from this validation run.

## Checks still pending

Linux execution and the race detector require the configured GitHub CI jobs.
This local Windows environment has CGO disabled and no C compiler, so those
checks have not been reported as passed. The project is ready to push for CI
validation; a release should wait for all required jobs to pass.
