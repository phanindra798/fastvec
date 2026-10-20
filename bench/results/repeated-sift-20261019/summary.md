# Repeated benchmark measurements

QPS is median [minimum, maximum] across separate process runs. Ranges are
observed variation, not confidence intervals. Recall is median [min, max].
Build time is excluded. hnswlib rebuilds each run, so variation includes
graph construction as well as timing. See session.json for run order and commands.

| ef | Fastvec QPS | hnswlib QPS | Fastvec recall | hnswlib recall |
|---:|---:|---:|---:|---:|
| 10 | 28,979 [26,833, 29,575] | 28,696 [28,194, 30,215] | 0.7099 [0.7099, 0.7099] | 0.7090 [0.7065, 0.7108] |
| 20 | 18,195 [17,323, 18,409] | 19,703 [18,945, 20,276] | 0.8395 [0.8395, 0.8395] | 0.8398 [0.8394, 0.8402] |
| 40 | 10,631 [10,444, 10,827] | 12,189 [12,186, 12,298] | 0.9275 [0.9275, 0.9275] | 0.9279 [0.9276, 0.9280] |
| 60 | 7,551 [7,481, 7,682] | 9,021 [8,529, 9,078] | 0.9599 [0.9599, 0.9599] | 0.9598 [0.9597, 0.9598] |
| 80 | 5,869 [5,797, 5,938] | 7,046 [6,979, 7,065] | 0.9754 [0.9754, 0.9754] | 0.9753 [0.9750, 0.9754] |
| 100 | 4,808 [4,737, 4,879] | 5,783 [3,964, 5,808] | 0.9835 [0.9835, 0.9835] | 0.9836 [0.9835, 0.9837] |
| 150 | 3,358 [3,317, 3,407] | 4,110 [3,971, 4,116] | 0.9929 [0.9929, 0.9929] | 0.9928 [0.9927, 0.9928] |
| 200 | 2,542 [2,454, 2,564] | 3,199 [3,079, 3,241] | 0.9963 [0.9963, 0.9963] | 0.9963 [0.9963, 0.9963] |
| 300 | 1,788 [1,756, 1,803] | 2,254 [2,247, 2,266] | 0.9987 [0.9987, 0.9987] | 0.9987 [0.9987, 0.9988] |
| 500 | 1,119 [1,101, 1,140] | 1,442 [1,399, 1,450] | 0.9997 [0.9997, 0.9997] | 0.9997 [0.9997, 0.9997] |

Session interruption: Pair 4 hnswlib attempt interrupted after a long output gap; on cancellation it reported build_seconds=1454.2 and partial ef10/20 results. No completed raw JSON was produced. Pairs 1–3 retained; pair 4 restarted and pair 5 completed afterward. Cause of delay unknown.
