"""Draw the recall/QPS chart from whatever is in bench/results.

One curve per implementation, swept over ef. This is the standard chart in
every ANN paper: recall on x, throughput on y, up and to the right is better.

    python py/plots.py
"""

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

    # A QPS number means nothing without the CPU it came off.
    fig.text(0.01, 0.01, first["env"]["cpu"], fontsize=6, color="#888")

    PLOTS.mkdir(parents=True, exist_ok=True)
    out = PLOTS / f"recall-qps-{dataset}.png"
    fig.tight_layout()
    fig.savefig(out)
    print(f"wrote {out}")


if __name__ == "__main__":
    for name in ("sift", "siftsmall"):
        plot(name)
