import unittest

from benchmark_timing import measure_throughput


class TimingTests(unittest.TestCase):
    def test_very_fast_sweeps(self):
        calls = 0
        def sweep():
            nonlocal calls
            calls += 1
        count, elapsed = measure_throughput(sweep, 1, lambda: calls * 50e-9)
        self.assertEqual(count, 1 << 21)
        self.assertGreaterEqual(elapsed, .1)

    def test_zero_duration_retries_and_counts_only_final_attempt(self):
        ticks = iter([0, 0, 0, 0.2])
        calls = []
        self.assertEqual(measure_throughput(lambda: calls.append(1), 3, lambda: next(ticks)), (6, 0.2))
        self.assertEqual(len(calls), 3)

    def test_stalled_clock(self):
        with self.assertRaisesRegex(RuntimeError, "resolution"):
            measure_throughput(lambda: None, 1, lambda: 0)

    def test_backwards_clock(self):
        ticks = iter([1, 0])
        with self.assertRaisesRegex(RuntimeError, "backwards"):
            measure_throughput(lambda: None, 1, lambda: next(ticks))

    def test_empty_queries(self):
        with self.assertRaises(ValueError):
            measure_throughput(lambda: self.fail("must not search"), 0)

    def test_search_failure(self):
        def fail():
            raise RuntimeError("search failed")
        with self.assertRaisesRegex(RuntimeError, "search failed"):
            measure_throughput(fail, 1)


if __name__ == "__main__":
    unittest.main()
