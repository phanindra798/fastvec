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

## Results

SIFT1M, one million 128-dimensional vectors, single thread, against hnswlib on
the same machine.

![recall against throughput](bench/plots/recall-qps-sift.png)

    ef      fastvec recall / QPS      hnswlib recall / QPS
    10      0.7099 /  5908            0.708  / 19907
    60      0.9599 /  1867            0.960  /  5286
    100     0.9835 /  1218            0.984  /  3771
    500     0.9997 /   287            0.9997 /   864

Recall matches to three decimal places at every point, so the graph is as good
as the reference implementation. Throughput is about 3x behind, and build is
15x slower: hnswlib inserts across all cores, this doesn't.

Exact search over the same data does 12 queries/sec at recall 1.000, so the
index is roughly 100x faster than brute force for 1.6% of answers differing at
ef=100.

Caveat on those throughput numbers: the machine was under load during the run
and the process averaged 29% CPU, so QPS is understated and the 3x gap is an
upper bound. Recall is unaffected. Needs a clean re-run, see
`docs/benchmarks.md`.

Full numbers in `docs/benchmarks.md`, reasoning in `docs/decisions.md`.

## Running

    make test
    make build

## Machine

Everything is measured on my laptop: i7-12650H, 16 GB RAM, Ubuntu 24.04 on
WSL2, Go 1.26.5. CPU only.

MIT licence.
