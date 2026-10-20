"""Draw the recall/QPS chart from whatever is in bench/results.

One curve per implementation, swept over ef. This is the standard chart in
every ANN paper: recall on x, throughput on y, up and to the right is better.

    python py/plots.py
"""

import argparse
import json
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

RESULTS = Path("bench/results")
PLOTS = Path("bench/plots")

# matplotlib's default cycle is hard to tell apart once printed.
COLOURS = ["#1c6b66", "#a8501a", "#3d5a80", "#6b4c9a", "#7a7a2e"]


def load(dataset):
    runs = []
    for path in sorted(RESULTS.glob(f"*-{dataset}.json")):
        with open(path) as f:
            runs.append(json.load(f))
    return runs


def plot(dataset):
    runs = load(dataset)
    if not runs:
        print(f"nothing for {dataset} in {RESULTS}")
        return

    fig, ax = plt.subplots(figsize=(7, 4.6), dpi=140)

    for i, run in enumerate(runs):
        sweep = run["sweep"]
        recalls = [p["recall"] for p in sweep]
        qps = [p["qps"] for p in sweep]
        ax.plot(
            recalls, qps,
            marker="o", markersize=4, linewidth=1.6,
            color=COLOURS[i % len(COLOURS)],
            label=run["implementation"],
        )

    ax.set_yscale("log")
    ax.set_xlabel(f"recall@{runs[0]['k']}")
    ax.set_ylabel("queries per second, single thread")
    ax.grid(True, which="both", linewidth=0.4, alpha=0.35)
    ax.legend(frameon=False)

    first = runs[0]
    ax.set_title(
        f"{dataset}  ·  {first['base_count']:,} x {first['dim']}  ·  "
        f"M={first['M']}, efConstruction={first['ef_construction']}",
        fontsize=9, loc="left", color="#555",
    )

    # Stamp the CPU on the chart so the numbers can be interpreted later.
    fig.text(0.01, 0.01, first["env"]["cpu"], fontsize=6, color="#888")

    PLOTS.mkdir(parents=True, exist_ok=True)
    out = PLOTS / f"recall-qps-{dataset}.png"
    fig.tight_layout()
    fig.savefig(out)
    print(f"wrote {out}")


def plot_repeated(session):
    """Plot medians and observed min/max ranges from a completed session."""
    metadata = json.loads((session / "session.json").read_text())
    if metadata["status"] != "complete":
        raise ValueError("cannot plot an incomplete benchmark session")
    summary = json.loads((session / "summary.json").read_text())
    fig, ax = plt.subplots(figsize=(8, 5), dpi=140)
    for i, (implementation, run) in enumerate(summary.items()):
        points = run["sweep"]
        x = [p["recall"]["median"] for p in points]
        y = [p["qps"]["median"] for p in points]
        def errors(field):
            return [[p[field]["median"] - p[field]["min"] for p in points],
                    [p[field]["max"] - p[field]["median"] for p in points]]
        ax.errorbar(x, y, xerr=errors("recall"), yerr=errors("qps"),
                    fmt="o-", markersize=4, capsize=3, linewidth=1.4,
                    color=COLOURS[i % len(COLOURS)], label=implementation)
    first = next(iter(summary.values()))
    ax.set_title(f"{first['dataset']} · {first['run_count']} paired runs · median and observed range", fontsize=11)
    ax.set_xlabel(f"recall@{first['k']}")
    ax.set_ylabel("queries per second, single thread")
    ax.set_yscale("log")
    ax.grid(True, which="both", linewidth=.4, alpha=.35)
    ax.legend(frameon=False)
    fig.text(.02, .015, "Bars show minimum–maximum, not confidence intervals. hnswlib rebuilds each run.", fontsize=8)
    fig.tight_layout(rect=(0, .04, 1, 1))
    path = session / "recall-qps.png"
    fig.savefig(path)
    plt.close(fig)
    print(f"wrote {path}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--session", type=Path, help="plot a completed repeated-run session")
    args = parser.parse_args()
    if args.session:
        plot_repeated(args.session)
    else:
        for name in ("sift", "siftsmall"):
            plot(name)
