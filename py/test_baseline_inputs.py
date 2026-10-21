import tempfile
import unittest
from pathlib import Path

import numpy as np

from bench_baselines import read_fvecs, read_ivecs, validate_inputs


class BaselineInputTests(unittest.TestCase):
    def test_invalid_records(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "bad.ivecs"
            for data in (b"", b"\x00", np.array([-1], dtype="<i4").tobytes(),
                         np.array([2, 1], dtype="<i4").tobytes(),
                         np.array([1, 0, 2, 0], dtype="<i4").tobytes()):
                with self.subTest(data=data):
                    path.write_bytes(data)
                    with self.assertRaises(ValueError):
                        read_ivecs(path)

    def test_valid_and_nonfinite_vectors(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "vectors.fvecs"
            for value in (1.25, float("nan"), float("inf")):
                path.write_bytes(np.array([1], dtype="<i4").tobytes()
                                 + np.array([value], dtype="<f4").tobytes())
                if np.isfinite(value):
                    self.assertEqual(read_fvecs(path).tolist(), [[1.25]])
                else:
                    with self.assertRaises(ValueError):
                        read_fvecs(path)

    def test_invalid_ground_truth_and_counts(self):
        base = np.zeros((2, 3), dtype=np.float32)
        queries = np.zeros((1, 3), dtype=np.float32)
        for truth, k in (([[-1]], 1), ([[2]], 1), ([[0]], 0),
                         ([[0]], 2), ([[0], [1]], 1)):
            with self.subTest(truth=truth, k=k):
                with self.assertRaises(ValueError):
                    validate_inputs(base, queries, np.array(truth), k)
        validate_inputs(base, queries, np.array([[0]]), 1)


if __name__ == "__main__":
    unittest.main()
