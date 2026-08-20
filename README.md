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

Exact search works and matches the SIFT1M ground truth exactly, recall@10 =
1.000 over 10,000 queries. It runs at 12 queries/sec on one thread, 60 across
sixteen.

For comparison, hnswlib on the same data and machine does 5,286 queries/sec at
96% recall. That's the gap HNSW is supposed to close.

Raw numbers are in `bench/results/`.

## Running

    make test
    make build

## Machine

Everything is measured on my laptop: i7-12650H, 16 GB RAM, Ubuntu 24.04 on
WSL2, Go 1.26.5. CPU only.

MIT licence.
