# fastvec

Vector similarity search engine, written from scratch in Go.

Implementing HNSW from the paper rather than calling a library, and benchmarking
it against hnswlib and FAISS on SIFT1M with everything running on a laptop CPU.

Early stage. Setting up the project.

## Plan

1. brute force search first, so I have something correct to compare against
2. HNSW on top of that
3. benchmark both against hnswlib on SIFT1M
4. then try to make it faster: mmap the index, AVX2 distance kernel, product
   quantization

## So far

Exact search matches the SIFT1M ground truth exactly, recall@10 = 1.000 over
10,000 queries, at 12 queries/sec on one thread.

HNSW is built through stage 3 of 4. On a 100k subset, single thread:

    Flat (exact)        recall 1.0000     92 QPS
    HNSW ef=100         recall 0.9961   2302 QPS

25x faster for 0.4% of the answers differing. Layers and the neighbour
heuristic are both in; scaling to the full million and comparing against
hnswlib is next.

Full numbers in `docs/benchmarks.md`, reasoning in `docs/decisions.md`.

## Running

    make test
    make build

## Machine

Everything is measured on my laptop: i7-12650H, 16 GB RAM, Ubuntu 24.04 on
WSL2, Go 1.26.5. CPU only.

MIT licence.
