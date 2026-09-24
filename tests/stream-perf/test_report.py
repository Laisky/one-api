"""Regression tests for paired evidence validation and aggregation."""
import copy
import unittest
import tempfile
from pathlib import Path
import json
import report
import run


def fixture() -> dict:
    """fixture creates three independent pairs with deliberately unequal baseline run speeds."""
    trials = []
    for repeat, speed in enumerate((10, 20, 100)):
        for label, factor in (('baseline', 1), ('candidate', 1.2)):
            trials.append({'label': label, 'repeat': repeat, 'profile': 'saturated', 'concurrency': 8,
                'offered': 128, 'completed': 128, 'failed': 0, 'dropped': 0, 'chunks': 32,
                'chunk_bytes': 128, 'pace_ms': 0, 'rate': 0, 'direct': False, 'binary_sha256': label,
                'billing': {'requests': 128, 'used_quota': 42}, 'successful_rps': speed * factor,
                'ttft_ms': {'p95': 1}, 'total_ms': {'p95': 2},
                'resources': {'gateway': {'cpu_ms_per_success': 10 / factor, 'peak_rss_mib': 100}}})
    return {'complete': True, 'trials': trials}


class ReportTests(unittest.TestCase):
    """ReportTests prevent incomplete, incomparable or incorrect traffic from being presented as a speedup."""
    def test_paired_run_ratios(self):
        """test_paired_run_ratios verifies run-level medians and preserves CPU direction."""
        cells = report.compare(fixture(), 3)
        self.assertEqual(len(cells), 1)
        self.assertAlmostEqual(cells[0]['metrics']['successful_rps']['paired_change_pct_median'], 20)
        self.assertAlmostEqual(cells[0]['metrics']['gateway_cpu_ms_per_success']['paired_change_pct_median'], -100 / 6)
        self.assertIn('not proof of statistical significance', report.markdown(cells))

    def test_atomic_progress_checkpoint(self):
        """test_atomic_progress_checkpoint preserves completion state and leaves no partial JSON in the canonical file."""
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            evidence = {'complete': False, 'trials': []}
            run.checkpoint(evidence, output)
            self.assertFalse(json.loads((output / 'summary.json').read_text())['complete'])
            evidence.update(complete=True, trials=[{'label': 'candidate'}])
            run.checkpoint(evidence, output)
            self.assertEqual(json.loads((output / 'summary.json').read_text()), evidence)
            self.assertFalse((output / 'summary.json.tmp').exists())

    def test_rejects_invalid_evidence(self):
        """test_rejects_invalid_evidence covers missing pairs, duplicates, drops, billing drift and binary drift."""
        mutations = (
            lambda s: s.update(complete=False),
            lambda s: s['trials'].pop(),
            lambda s: s['trials'].append(copy.deepcopy(s['trials'][0])),
            lambda s: s['trials'][0].update(dropped=1),
            lambda s: s['trials'][0].update(failed=1),
            lambda s: s['trials'][0].update(offered=129),
            lambda s: s['trials'][0].update(chunks=33),
            lambda s: s['trials'][0].update(binary_sha256='changed'),
            lambda s: s['trials'][0]['billing'].update(used_quota=43),
            lambda s: s['trials'][0]['billing'].update(requests=127),
            lambda s: s['trials'][0].update(successful_rps=float('nan')),
        )
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                evidence = fixture()
                mutation(evidence)
                with self.assertRaises(ValueError):
                    report.compare(evidence, 3)


if __name__ == '__main__':
    unittest.main()
