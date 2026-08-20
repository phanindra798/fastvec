"""Run hnswlib on SIFT and record recall/QPS, so there's a target to aim at.

Recall is measured by distance, matching the Go side. See decisions.md.

    python py/bench_baselines.py --data /mnt/d/data/sift --name sift
"""

import argparse
import json
import platform
import subprocess
import time
from pathlib import Path

import hnswlib
import numpy as np


def read_fvecs(path):
    raw = np.fromfile(path, dtype=np.int32)
    dim = raw[0]
    return raw.reshape(-1, dim + 1)[:, 1:].view(np.float32)


def read_ivecs(path):
    raw = np.fromfile(path, dtype=np.int32)
    dim = raw[0]
    return raw.reshape(-1, dim + 1)[:, 1:]


def recall_at(base, queries, truth, found_ids, k):
    """Fraction of returned results that are within the k'th true distance.

    Threshold per query is the distance to the furthest of the k listed
    neighbours. Anything at or under that is a valid k-nearest answer, whichever
    ID it happens to have.
    """
    hits = 0
    for q in range(len(queries)):
        gt = base[truth[q][:k]]
        d = ((gt - queries[q]) ** 2).sum(axis=1)
        threshold = d.max()

        got = base[found_ids[q]]
        dg = ((got - queries[q]) ** 2).sum(axis=1)
        hits += int((dg <= threshold).sum())

    return hits / (len(queries) * k)


def cpu_model():
    for line in Path("/proc/cpuinfo").read_text().splitlines():
        if line.startswith("model name"):
            return line.split(":", 1)[1].strip()
    return "unknown"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default="/mnt/d/data/sift")
    ap.add_argument("--name", default="sift")
    ap.add_argument("--k", type=int, default=10)
    ap.add_argument("--M", type=int, default=16)
    ap.add_argument("--ef-construction", type=int, default=200)
    ap.add_argument("--out", default="bench/results")
    args = ap.parse_args()

    d = Path(args.data)
    base = read_fvecs(d / f"{args.name}_base.fvecs")
    queries = read_fvecs(d / f"{args.name}_query.fvecs")
    truth = read_ivecs(d / f"{args.name}_groundtruth.ivecs")
    n, dim = base.shape
    print(f"base {n} x {dim}, {len(queries)} queries")

    index = hnswlib.Index(space="l2", dim=dim)
    index.init_index(max_elements=n, M=args.M, ef_construction=args.ef_construction, random_seed=42)

    # Build uses every core. Query timing below is single threaded, which is the
    # ann-benchmarks convention and what our Go single thread number compares to.
    t0 = time.perf_counter()
    index.add_items(base, np.arange(n))
    build_secs = time.perf_counter() - t0
    print(f"built in {build_secs:.1f}s")

    index.set_num_threads(1)

    results = []
    for ef in [10, 20, 40, 60, 80, 100, 150, 200, 300, 500]:
        if ef < args.k:
            continue
        index.set_ef(ef)

        index.knn_query(queries[:300], k=args.k)  # warm up

        t0 = time.perf_counter()
        ids, _ = index.knn_query(queries, k=args.k)
        elapsed = time.perf_counter() - t0

        r = recall_at(base, queries, truth, ids, args.k)
        qps = len(queries) / elapsed
        results.append({"ef": ef, "recall": r, "qps": qps, "ms_per_query": 1000 * elapsed / len(queries)})
        print(f"  ef={ef:<4} recall@{args.k}={r:.4f}  {qps:8.1f} QPS  {1000*elapsed/len(queries):.3f} ms")

    try:
        commit = subprocess.check_output(["git", "rev-parse", "--short", "HEAD"], text=True).strip()
    except Exception:
        commit = "unknown"

    out = {
        "implementation": "hnswlib",
        "version": hnswlib.__version__ if hasattr(hnswlib, "__version__") else "0.8.0",
        "dataset": args.name,
        "base_count": int(n),
        "dim": int(dim),
        "queries": int(len(queries)),
        "k": args.k,
        "M": args.M,
        "ef_construction": args.ef_construction,
        "build_seconds": build_secs,
        "threads": 1,
        "sweep": results,
        "env": {
            "cpu": cpu_model(),
            "python": platform.python_version(),
            "numpy": np.__version__,
            "kernel": platform.release(),
            "commit": commit,
        },
    }

    outdir = Path(args.out)
    outdir.mkdir(parents=True, exist_ok=True)
    path = outdir / f"hnswlib-{args.name}.json"
    path.write_text(json.dumps(out, indent=2))
    print(f"wrote {path}")


if __name__ == "__main__":
    main()
