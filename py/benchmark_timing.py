"""Search-only throughput timing, matching the Go runner's sweep policy."""

import time


def measure_throughput(sweep, queries, clock=time.perf_counter):
    if queries < 1:
        raise ValueError("throughput measurement needs at least one query")
    repeats = 1
    while repeats <= 1 << 24:
        start = clock()
        for _ in range(repeats):
            sweep()
        elapsed = clock() - start
        if elapsed < 0:
            raise RuntimeError("benchmark clock moved backwards")
        if elapsed >= 0.1:
            return repeats * queries, elapsed
        repeats *= 2
    raise RuntimeError("benchmark clock resolution is insufficient after repeated sweeps")
