"""Guard the compact final-run ledger without mistaking it for raw qualification."""
import copy
import csv
import importlib.util
import io
from pathlib import Path
import unittest

ROOT = Path(__file__).with_name('results') / '20260925-byte-budget'
SPEC = importlib.util.spec_from_file_location('byte_budget_verifier', ROOT / 'verify.py')
audit = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(audit)


class ByteBudgetEvidenceTests(unittest.TestCase):
    """ByteBudgetEvidenceTests reject omissions, corruption and harmful synthetic changes."""

    def setUp(self):
        """setUp loads the immutable committed observations for each independent negative control."""
        self.rows = list(csv.DictReader(io.StringIO((ROOT / 'final-runs.csv').read_text())))

    def test_committed_evidence(self):
        """test_committed_evidence reproduces acceptance without claiming raw request verification."""
        result = audit.verify(ROOT)
        self.assertTrue(result['performance_gates_pass'])
        self.assertFalse(result['raw_qualification_reverified'])
        self.assertEqual(result['trials'], 40)

    def test_missing_and_duplicate_runs(self):
        """test_missing_and_duplicate_runs rejects both single-row and whole-cell removal."""
        for rows in [self.rows[:-1], self.rows + self.rows[:1], [r for r in self.rows if r['profile'] != 'paced']]:
            with self.assertRaises(ValueError):
                audit.compare(rows)

    def test_invalid_metrics(self):
        """test_invalid_metrics rejects nonfinite and negative durations instead of reporting gains."""
        for value in ('nan', 'inf', '-1'):
            rows = copy.deepcopy(self.rows)
            rows[0]['cpu_ms'] = value
            with self.assertRaises(ValueError):
                audit.compare(rows)

    def test_latency_regression_rejects(self):
        """test_latency_regression_rejects preserves both relative and absolute latency thresholds."""
        for row in self.rows:
            if row['variant'] == 'candidate' and row['profile'] == 'saturated':
                row['ttft_p95_ms'] = '100000'
        self.assertFalse(audit.compare(self.rows)['performance_gates_pass'])

    def test_rss_regression_rejects(self):
        """test_rss_regression_rejects prevents throughput gains from excusing excessive resident growth."""
        for row in self.rows:
            if row['variant'] == 'candidate':
                row['rss_mib'] = '100000'
        self.assertFalse(audit.compare(self.rows)['performance_gates_pass'])

    def test_no_material_gain_rejects(self):
        """test_no_material_gain_rejects requires a repeat-supported benefit, not just no harm."""
        for row in self.rows:
            if row['variant'] == 'candidate':
                baseline = next(r for r in self.rows if r['variant'] == 'baseline' and
                                all(r[k] == row[k] for k in ('profile', 'concurrency', 'repeat')))
                for metric in audit.METRICS:
                    row[metric] = baseline[metric]
        self.assertFalse(audit.compare(self.rows)['performance_gates_pass'])
