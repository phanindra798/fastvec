# Benchmarks

All measured on one laptop: i7-12650H, 6 P-cores + 4 E-cores, 16 GB, Ubuntu
24.04 on WSL2, Go 1.26.5. CPU only. Absolute numbers will differ elsewhere;
everything compared here was run in the same session.

Datasets: SIFT-100k for the stage gates (100k base, 1000 queries, ground truth
computed by Flat), SIFT1M for the headline figures. k=10 throughout.

Distance counts come from a `-tags diagnostic` build. Timings never do.

## Exact search

Flat, 100k x 128, single thread, three runs: 8.98 / 8.41 / 8.28 ms.

    workers   1      2      4      6      8      12     16
    ms      7.97   3.94   2.44   1.95   2.21   1.92   1.75

8 workers beats 12 and loses to 6. Six P-cores, four E-cores, and the merge
waits for the slowest. 4.56x from 16 threads, not 16x: each query moves 51 MB in
1.75 ms, about 29 GB/s, near what the memory delivers. That also caps what AVX2
can buy later.

SIFT1M: 10,000 queries in 2m45s, 16.5 ms each, 60 QPS on 16 threads, 12 on one.
Recall 1.0000 against the shipped ground truth.

## hnswlib, the target

M=16, efConstruction=200, single thread, SIFT1M. Build 92.6s.

    ef        10      40      60     100     200     500
    recall  0.708   0.928   0.960   0.984   0.996   0.9997
    QPS     19907    7650    5286    3771    1999      864

5286 QPS at 96% recall against our exact 12, so roughly 440x for 4% of the
answers differing.

## Stage 1, flat graph

Pruning at M instead of MMax orphaned nearly a quarter of the graph:

    prune at        M          2M
    orphans     23,214          4
    recall       0.6084     0.9863

Sweep with MMax = 2M, SIFT-100k:

    ef        10      20      50     100     200
    recall  0.682   0.837   0.950   0.986   0.996
    QPS     10248    6755    3744    2340    1393

Build 48.6s for 100k. 2340 QPS at ef=100 against Flat's 92.

## Stage 2, layers

Level sizes [100000 6201 397 17 2], shrinking by roughly M each step up.

    ef=20    single 0.8369, 550 dist    layered 0.8550, 481 dist
    ef=50    single 0.9500, 909 dist    layered 0.9470, 831 dist
    ef=100   single 0.9863, 1407 dist   layered 0.9756, 1327 dist

Layers touch 6-13% fewer nodes. They also broke the graph:

    single layer   5 components, largest 99,996 of 100,000   ( 0.0% stranded)
    layered        6 components, largest 54,217 of 100,000   (45.8% stranded)

Same data, same seed, only the hierarchy differs. Recall survived at 0.9756
because the components are spatially coherent and the descent drops a query into
the region its neighbours live in. Queries near a boundary lose the neighbours
across it, which is the 1% gap against the flat build.

## Stage 3, neighbour heuristic

                     components   stranded   build
    nearest-M          6           45.8%     50.4s
    heuristic          1            0.0%     56.1s

    ef      nearest-M                  heuristic
    20      0.8550,  481 dist          0.9046,  459 dist
    50      0.9470,  831 dist          0.9793,  851 dist
    100     0.9756, 1327 dist          0.9961, 1421 dist

One component holding all 100,000 nodes. Recall at ef=100 went 0.9756 to 0.9961,
past the flat build's 0.9863.

At equal ef the heuristic touches 7% more nodes. At equal recall it touches far
fewer: nearest-M never reaches 0.979 anywhere in the sweep, its best being
0.9756 at 1327 distances, and the heuristic beats that at ef=50 using 851. Same
accuracy, 36% less work.

Degree min fell from 16 to 2, mean from 25.0 to 19.8. The heuristic rejects
candidates, so the graph came out sparser and better connected at once. Build
cost 11%.

## Where it stands

SIFT-100k, k=10, single thread:

                        recall    QPS    dist/query
    Flat (exact)        1.0000     92      100,000
    stage 1 flat graph  0.9863   2340        1,407
    stage 2 layered     0.9756   2563        1,327
    stage 3 heuristic   0.9961   2302        1,421

SIFT1M numbers, and the comparison against hnswlib on equal terms, are stage 4.
