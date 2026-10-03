"""Keep failed billing trials identifiable without weakening admission or exposing fixture credentials."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import run


class FailureEvidenceTests(unittest.TestCase):
    """FailureEvidenceTests verify exact accounting gates and durable incomplete-study checkpoints."""

    def test_wait_usage_reports_timeout_counters(self):
        """test_wait_usage_reports_timeout_counters preserves the observed count rather than reporting success."""
        with mock.patch.object(run, 'usage_snapshot', return_value=(120, 9)), \
             mock.patch.object(run.time, 'monotonic', side_effect=[0, 10]):
            with self.assertRaises(run.BillingMismatch) as caught:
                run.wait_usage(Path('/unused'), 10)
        self.assertEqual(caught.exception.evidence['expected_requests'], 10)
        self.assertEqual(caught.exception.evidence['observed_requests'], 9)
        self.assertEqual(caught.exception.evidence['observed_used_quota'], 120)

    def test_wait_usage_rejects_duplicate_and_accepts_exact_counts(self):
        """test_wait_usage_rejects_duplicate_and_accepts_exact_counts checks both sides of the equality gate."""
        with mock.patch.object(run, 'usage_snapshot', return_value=(120, 11)):
            with self.assertRaisesRegex(run.BillingMismatch, 'more billed'):
                run.wait_usage(Path('/unused'), 10)
        with mock.patch.object(run, 'usage_snapshot', return_value=(120, 10)):
            self.assertEqual(run.wait_usage(Path('/unused'), 10), (120, 10))

    def test_failed_trial_is_checkpointed_without_credentials(self):
        """test_failed_trial_is_checkpointed_without_credentials keeps completed trials and failing coordinates separate."""
        for failure in (run.BillingMismatch(272, (123, 271), 'unsettled'), RuntimeError('secret-must-not-be-published')):
            with self.subTest(error=type(failure).__name__), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                binary = root / 'binary'
                binary.write_bytes(b'fixture binary, never executed')
                output = root / 'result'
                argv = ['run.py', '--binary', str(binary), '--driver', str(binary), '--token-cache', str(root),
                        '--output', str(output), '--concurrency', '8,64', '--profiles', 'saturated', '--repeats', '1']
                with mock.patch('sys.argv', argv), mock.patch.object(run, 'qualify', return_value={'normal': 0}), \
                     mock.patch.object(run, 'measure', side_effect=[{'completed': 128}, failure]):
                    with self.assertRaises(type(failure)):
                        run.main()
                saved = json.loads((output / 'summary.json').read_text())
                self.assertIs(saved['complete'], False)
                self.assertEqual(saved['trials'], [{'completed': 128}])
                self.assertEqual(saved['failure']['concurrency'], 64)
                self.assertEqual(saved['failure']['label'], 'candidate')
                self.assertEqual(saved['failure']['phase'], 'measurement')
                self.assertIn('finished_at_utc', saved)
                self.assertNotIn('secret-must-not-be-published', json.dumps(saved))
                if isinstance(failure, run.BillingMismatch):
                    self.assertEqual(saved['failure']['billing']['observed_requests'], 271)
                else:
                    self.assertNotIn('billing', saved['failure'])
                self.assertFalse((output / 'summary.json.tmp').exists())
