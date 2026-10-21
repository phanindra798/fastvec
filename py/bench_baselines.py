"""Run hnswlib on SIFT and record recall/QPS, so there's a target to aim at.

Recall is measured by distance, matching the Go side. See decisions.md.

    python py/bench_baselines.py --data /mnt/d/data/sift --name sift
"""

import argparse
from datetime import datetime, timezone
import json
import platform
import subprocess
import time
from pathlib import Path

import numpy as np

from benchmark_timing import measure_throughput
from benchmark_metadata import package_version


def read_fvecs(path):
    values = read_ivecs(path).view("<f4")
    if not np.isfinite(values).all():
        raise ValueError(f"{path}: vectors contain non-finite values")
    return values


def read_ivecs(path):
    if Path(path).stat().st_size % 4:
        raise ValueError(f"{path}: truncated vecs word")
    raw = np.fromfile(path, dtype="<i4")
    if not raw.size:
        raise ValueError(f"{path}: empty vecs file")
    dim = int(raw[0])
    if not 1 <= dim <= 65536 or raw.size % (dim + 1):
        raise ValueError(f"{path}: invalid dimension or truncated record")
    records = raw.reshape(-1, dim + 1)
    if not np.all(records[:, 0] == dim):
        raise ValueError(f"{path}: inconsistent record dimensions")
    return records[:, 1:]


def validate_inputs(base, queries, truth, k):
    if base.shape[1] != queries.shape[1] or len(queries) != len(truth):
        raise ValueError("base, query and ground-truth dimensions/counts disagree")
    if not 1 <= k <= min(len(base), truth.shape[1], 500):
        raise ValueError("k must fit the base, ground truth and ef sweep (1..500)")
    if np.any(truth < 0) or np.any(truth >= len(base)):
        raise ValueError("ground truth contains out-of-range vector IDs")


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
    if platform.system() == "Windows":
        import winreg
        try:
            with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, r"HARDWARE\DESCRIPTION\System\CentralProcessor\0") as key:
                return winreg.QueryValueEx(key, "ProcessorNameString")[0].strip()
        except OSError:
            return platform.processor() or "unknown"
    try:
        for line in Path("/proc/cpuinfo").read_text().splitlines():
            if line.startswith("model name"):
                return line.split(":", 1)[1].strip()
    except OSError:
        pass
    return platform.processor() or "unknown"


def main():
    import hnswlib

    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default="/mnt/d/data/sift")
    ap.add_argument("--name", default="sift")
    ap.add_argument("--k", type=int, default=10)
    ap.add_argument("--M", type=int, default=16)
    ap.add_argument("--ef-construction", type=int, default=200)
    ap.add_argument("--out", default="bench/results")
    args = ap.parse_args()
    if Path(args.name).name != args.name or args.name in ("", ".", ".."):
        ap.error("--name must be a dataset filename prefix")
    if args.M < 2 or args.ef_construction < args.M:
        ap.error("--M must be at least 2 and --ef-construction must be at least M")
    started_utc = datetime.now(timezone.utc).isoformat()

    d = Path(args.data)
    base = read_fvecs(d / f"{args.name}_base.fvecs")
    queries = read_fvecs(d / f"{args.name}_query.fvecs")
    truth = read_ivecs(d / f"{args.name}_groundtruth.ivecs")
    validate_inputs(base, queries, truth, args.k)
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

        timed_queries, elapsed = measure_throughput(
            lambda: index.knn_query(queries, k=args.k), len(queries)
        )
        # Evaluate recall in a separate, untimed pass, matching the Go runner.
        ids, _ = index.knn_query(queries, k=args.k)

        r = recall_at(base, queries, truth, ids, args.k)
        qps = timed_queries / elapsed
        results.append({"ef": ef, "recall": r, "qps": qps, "ms_per_query": 1000 * elapsed / timed_queries,
                        "timed_queries": timed_queries, "timed_seconds": elapsed})
        print(f"  ef={ef:<4} recall@{args.k}={r:.4f}  {qps:8.1f} QPS  {1000*elapsed/timed_queries:.3f} ms")

    try:
        commit = subprocess.check_output(["git", "rev-parse", "--short", "HEAD"], text=True).strip()
        source_state = "dirty" if subprocess.check_output(["git", "status", "--porcelain"], text=True).strip() else "clean"
    except Exception:
        commit = "unknown"
        source_state = "unknown"

    out = {
        "timing_method": "search_only_repeated_sweeps_v2",
        "implementation": "hnswlib",
        "version": package_version(hnswlib, "hnswlib"),
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
            "captured_utc": started_utc,
            "source_state": source_state,
            "os": platform.system() + "/" + platform.machine(),
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
