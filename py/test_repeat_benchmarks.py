import copy
import unittest

from repeat_benchmarks import summarize, paired_summary


def run(qps=100):
    return dict(timing_method='search_only_repeated_sweeps_v2', implementation='fastvec',
                dataset='tiny', base_count=100, dim=2, queries=10, k=10, M=16,
                ef_construction=200, threads=1,
                sweep=[dict(ef=10, qps=qps, recall=.9, timed_queries=100,
                            timed_seconds=100/qps)])


class SummaryTests(unittest.TestCase):
    def test_load_policy_is_retained_and_cannot_be_mixed(self):
        runs = [run(), run(), run()]
        for item in runs:
            item['index_load_policy'] = 'single-pass'
        self.assertEqual(summarize(runs)['index_load_policy'], 'single-pass')
        runs[-1]['index_load_policy'] = 'auto'
        with self.assertRaisesRegex(ValueError, 'index_load_policy'):
            summarize(runs)

    def test_outlier_does_not_replace_median(self):
        point = summarize([run(100), run(120), run(1000)])['sweep'][0]['qps']
        self.assertEqual(point, dict(median=120, min=100, max=1000, samples=[100,120,1000]))

    def test_rejects_incomparable_or_invalid_runs(self):
        for key, value in [('dataset','other'), ('threads',2), ('timing_method','old')]:
            runs = [run(),run(),run()]
            runs[-1][key] = value
            with self.subTest(key=key), self.assertRaises(ValueError): summarize(runs)
        for key, value in [('qps',float('nan')), ('recall',1.1), ('timed_seconds',0),
                           ('timed_queries',99), ('ef',20), ('qps',200)]:
            runs = [run(),run(),run()]
            runs[-1]['sweep'][0][key] = value
            with self.subTest(key=key), self.assertRaises(ValueError): summarize(runs)

    def test_requires_three_observations(self):
        with self.assertRaises(ValueError): summarize([run(),run()])

    def test_pair_rejects_different_datasets(self):
        pairs = []
        for _ in range(3):
            a,b = run(),run()
            b['implementation'] = 'hnswlib'
            pairs.append(dict(fastvec=a,hnswlib=b))
        self.assertEqual(paired_summary(pairs)['hnswlib']['run_count'],3)
        bad = copy.deepcopy(pairs)
        bad[0]['hnswlib']['dataset'] = 'other'
        with self.assertRaises(ValueError): paired_summary(bad)


if __name__ == '__main__':
    unittest.main()
