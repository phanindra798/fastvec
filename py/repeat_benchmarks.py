"""Run paired benchmarks sequentially and retain every raw observation.

Requires a prebuilt fastvec-bench executable and the baseline dependencies.
The output directory must be new, preventing accidental mixing of sessions.
"""

import argparse
from datetime import datetime, timezone
import json
import math
from pathlib import Path
import statistics
import subprocess
import sys


MATCH_KEYS = ("timing_method", "dataset", "base_count", "dim", "queries",
              "k", "M", "ef_construction", "threads")


def summarize(runs):
    """Summarize independent process runs, never pooled query durations."""
    if len(runs) < 3:
        raise ValueError("at least three runs are required")
    first = runs[0]
    if first["timing_method"] != "search_only_repeated_sweeps_v2":
        raise ValueError("unsupported timing method")
    efs = [p["ef"] for p in first["sweep"]]
    if not efs or len(set(efs)) != len(efs):
        raise ValueError("empty or duplicate ef sweep")
    for run in runs:
        if run.get("index_load_policy") != first.get("index_load_policy"):
            raise ValueError("incompatible index_load_policy")
        for key in (*MATCH_KEYS, "implementation"):
            if run[key] != first[key]:
                raise ValueError(f"incompatible {key}")
        if [p["ef"] for p in run["sweep"]] != efs:
            raise ValueError("incompatible ef sweep")
        for p in run["sweep"]:
            if not all(math.isfinite(p[k]) for k in ("qps", "recall", "timed_seconds")):
                raise ValueError("non-finite measurement")
            if p["qps"] <= 0 or not 0 <= p["recall"] <= 1 or p["timed_seconds"] < .1:
                raise ValueError("invalid measurement")
            count = p["timed_queries"]
            if not isinstance(count, int) or count <= 0 or count % run["queries"]:
                raise ValueError("invalid timed query count")
            if not math.isclose(p["qps"], count / p["timed_seconds"], rel_tol=1e-8):
                raise ValueError("inconsistent throughput")
    sweep = []
    for i, ef in enumerate(efs):
        point = {"ef": ef}
        for field in ("qps", "recall"):
            values = [run["sweep"][i][field] for run in runs]
            point[field] = {"median": statistics.median(values),
                            "min": min(values), "max": max(values), "samples": values}
        sweep.append(point)
    return {**{key: first[key] for key in MATCH_KEYS},
            "implementation": first["implementation"], "run_count": len(runs),
            "index_load_policy": first.get("index_load_policy"),
            "sweep": sweep}


def paired_summary(pairs):
    for pair in pairs:
        for key in MATCH_KEYS:
            if pair["fastvec"][key] != pair["hnswlib"][key]:
                raise ValueError(f"pair has incompatible {key}")
        if [p["ef"] for p in pair["fastvec"]["sweep"]] != [p["ef"] for p in pair["hnswlib"]["sweep"]]:
            raise ValueError("pair has incompatible ef sweep")
    return {impl: summarize([pair[impl] for pair in pairs]) for impl in ("fastvec", "hnswlib")}


def markdown(summary):
    lines = ["# Repeated benchmark measurements", "",
             "QPS is median [minimum, maximum] across separate process runs. Ranges are",
             "observed variation, not confidence intervals. Recall is median [min, max].",
             "Build time is excluded. hnswlib rebuilds each run, so variation includes",
             "graph construction as well as timing. See session.json for run order and commands.", "",
             "| ef | Fastvec QPS | hnswlib QPS | Fastvec recall | hnswlib recall |",
             "|---:|---:|---:|---:|---:|"]
    for a, b in zip(summary["fastvec"]["sweep"], summary["hnswlib"]["sweep"]):
        def cell(p, field):
            v = p[field]
            fmt = ',.0f' if field == 'qps' else '.4f'
            return f"{v['median']:{fmt}} [{v['min']:{fmt}}, {v['max']:{fmt}}]"
        lines.append(f"| {a['ef']} | {cell(a,'qps')} | {cell(b,'qps')} | {cell(a,'recall')} | {cell(b,'recall')} |")
    return '\n'.join(lines) + '\n'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--fastvec', required=True, type=Path, help='prebuilt executable')
    parser.add_argument('--data', required=True, type=Path)
    parser.add_argument('--name', default='sift')
    parser.add_argument('--runs', type=int, default=5)
    parser.add_argument('--index', type=Path, help='optional Fastvec index cache')
    parser.add_argument('--index-load-policy', choices=('auto', 'single-pass'), default='auto',
                        help='Fastvec cache loading policy; ignored for newly built indexes')
    parser.add_argument('--out', required=True, type=Path, help='new session directory')
    args = parser.parse_args()
    if args.runs < 3:
        parser.error('--runs must be at least 3')
    if Path(args.name).name != args.name or args.name in ('', '.', '..'):
        parser.error('--name must be a dataset filename prefix')
    if not args.fastvec.is_file():
        parser.error('--fastvec must be an existing executable')
    root = Path(__file__).resolve().parent.parent
    args.out = args.out.resolve()
    args.data = args.data.resolve()
    executable = args.fastvec.resolve()
    args.out.mkdir(parents=True, exist_ok=False)
    session = {"started_utc": datetime.now(timezone.utc).isoformat(),
               "status": "running", "runs": args.runs, "commands": []}
    def save_session():
        (args.out / 'session.json').write_text(json.dumps(session, indent=2)+'\n', encoding='utf-8')
    pairs = []
    try:
        for i in range(args.runs):
            pair = {}
            order = ('fastvec', 'hnswlib') if i % 2 == 0 else ('hnswlib', 'fastvec')
            out = args.out / f'run-{i+1:02d}'
            for impl in order:
                if impl == 'fastvec':
                    command = [str(executable), '-data', str(args.data), '-name', args.name, '-out', str(out)]
                    command += ['-index-load-policy', args.index_load_policy]
                    if args.index:
                        command += ['-index', str(args.index.resolve())]
                else:
                    command = [sys.executable, '-u', str(root/'py/bench_baselines.py'), '--data', str(args.data), '--name', args.name, '--out', str(out)]
                session['commands'].append({"run": i+1, "implementation": impl, "argv": command})
                save_session()
                print(f'Run {i+1}/{args.runs}: {impl}', flush=True)
                subprocess.run(command, cwd=root, check=True)
                pair[impl] = json.loads((out/f'{impl}-{args.name}.json').read_text())
            pairs.append(pair)
        summary = paired_summary(pairs)
        (args.out/'summary.json').write_text(json.dumps(summary, indent=2, allow_nan=False)+'\n', encoding='utf-8')
        (args.out/'summary.md').write_text(markdown(summary), encoding='utf-8')
        session['status'] = 'complete'
    except BaseException:
        session['status'] = 'failed'
        raise
    finally:
        save_session()


if __name__ == '__main__':
    main()
