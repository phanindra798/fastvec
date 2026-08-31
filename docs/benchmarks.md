# Benchmarks

All measured on one laptop: i7-12650H, 6 P-cores + 4 E-cores, 16 GB, Ubuntu
24.04 on WSL2, Go 1.26.5. CPU only. Absolute numbers will differ elsewhere;
everything compared here was run in the same session.

Datasets: SIFT-100k for the stage gates (100k base, 1000 queries, ground truth
computed by Flat), SIFT1M for the headline figures. k=10 throughout.

Distance counts come from a `-tags diagnostic` build. Timings never do.

## Headline, SIFT1M

Both implementations run back to back in one session, machine otherwise idle.
Load average 0.45 at the start; fastvec held 99% of one core throughout, so
nothing was competing. M=16, efConstruction=200, single thread.

    ef      fastvec recall / QPS      hnswlib recall / QPS      ratio
    10      0.7099 / 13079            0.7080 / 22858            1.75x
    20      0.8395 /  8573            0.8402 / 14756            1.72x
    40      0.9275 /  5854            0.9278 /  8994            1.54x
    60      0.9599 /  4109            0.9600 /  6282            1.53x
    80      0.9754 /  3237            0.9754 /  4824            1.49x
    100     0.9835 /  2656            0.9836 /  4234            1.59x
    150     0.9929 /  1846            0.9928 /  2973            1.61x
    200     0.9963 /  1415            0.9963 /  2244            1.59x
    300     0.9987 /  1073            0.9988 /  1515            1.41x
    500     0.9997 /   626            0.9997 /   937            1.50x

Recall matches to three or four decimal places at every point. The graph is as
good as the reference implementation; the gap is execution speed, 1.5 to 1.75x.

Exact search over the same data does 12 QPS at recall 1.0000, so the index is
roughly 220x faster than brute force at ef=100 for 1.6% of answers differing.

### Build

    fastvec   11m12s at   99% CPU  =  672 core-seconds
    hnswlib     83.1s at 1100% CPU  =  914 core-seconds

hnswlib is 8x faster on the clock because it inserts across eleven cores. Per
core it does 26% more work than this build does. The wall-clock gap is entirely
parallelism, not inefficiency, and parallel insert is the obvious next step.

Peak RSS 1.21 GB. Levels [1000000 62356 3994 246 26 2], degree min=1 max=32
mean=21.1, and the bottom layer is one reachable set covering all 1,000,000.

### An earlier run of this was wrong

The first SIFT1M measurement reported 3x slower than hnswlib. That run averaged
29% CPU over 86 minutes because something else was using the machine, and it
predated the AVX2 kernel. It also compared against hnswlib numbers taken ten
days earlier under unknown conditions.

Both of those are why this one runs the two implementations minutes apart on an
idle machine and records load average and CPU percentage alongside the results.

## Exact search

Flat, 100k x 128, single thread, three runs: 8.98 / 8.41 / 8.28 ms.

    workers   1      2      4      6      8      12     16
    ms      7.97   3.94   2.44   1.95   2.21   1.92   1.75

8 workers beats 12 and loses to 6. Six P-cores, four E-cores, and the merge
waits for the slowest. 4.56x from 16 threads, not 16x: each query moves 51 MB in
1.75 ms, about 29 GB/s, near what the memory delivers.

SIFT1M: 10,000 queries in 2m45s, 16.5 ms each, 60 QPS on 16 threads, 12 on one.
Recall 1.0000 against the shipped ground truth.

## Stage 1, flat graph

Pruning at M instead of MMax orphaned nearly a quarter of the graph:

    prune at        M          2M
    orphans     23,214          4
    recall       0.6084     0.9863

Sweep with MMax = 2M, SIFT-100k:

    ef        10      20      50     100     200
    recall  0.682   0.837   0.950   0.986   0.996
    QPS     10248    6755    3744    2340    1393

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
candidates, so the graph came out sparser and better connected at once.

## AVX2 distance kernel

Same public function, matched benchmarks, two builds. `-tags purego` compiles
the assembly out entirely.

    scalar   80.09 / 81.50 / 81.74 / 82.17 / 82.41 ns
    avx2     12.25 / 12.57 / 12.71 / 12.72 / 14.07 ns

6.4x on the kernel, median to median, 128 dimensions.

End to end on SIFT-100k, same index, k=10:

    ef       scalar QPS   avx2 QPS   speedup   recall
    20            7,288     14,475     1.99x   0.9046 both
    100           2,020      3,788     1.88x   0.9961 both
    200           1,152      2,135     1.85x   0.9992 both

Recall identical at every point, so the kernel changed speed and nothing else.

6.4x on one part turning into 1.88x overall puts that part at about 55% of
search time before, roughly 15% after. The rest is heap operations, visited set
checks, and cache misses walking the graph.

Correctness: worst relative error against scalar was 1.64e-06 over 6500 random
pairs across 14 dimension sizes. Across 2000 trials the kernel never disagreed
with scalar about which of two vectors was nearer, which is the property that
actually matters.

## Index persistence

    save   58 MB for the 100k index
    load   164 ms

Before the buffered word reader it was 613 ms, because binary.Read was being
called once per value and SIFT1M holds about 150 million of them.

SIFT1M loads in roughly 2 seconds against an 11 minute rebuild, which is what
makes re-measuring on an idle machine practical.

## SIFT-100k summary

k=10, single thread:

                        recall    QPS    dist/query
    Flat (exact)        1.0000     92      100,000
    stage 1 flat graph  0.9863   2340        1,407
    stage 2 layered     0.9756   2563        1,327
    stage 3 heuristic   0.9961   2302        1,421
