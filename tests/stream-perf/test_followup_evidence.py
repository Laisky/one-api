"""Negative controls for the committed follow-up performance ledger."""
from __future__ import annotations
import copy
import importlib.util
from pathlib import Path
import tempfile
import unittest

PATH = Path(__file__).parent / 'results' / '20260924-followup' / 'verify.py'
SPEC = importlib.util.spec_from_file_location('stream_followup_verifier', PATH)
VERIFY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(VERIFY)


class FollowupEvidenceTests(unittest.TestCase):
    """FollowupEvidenceTests prevent incomplete or corrupted evidence from being accepted."""

    def setUp(self):
        """setUp loads one independent copy of the verified manifest and complete run ledger."""
        self.manifest, self.rows = VERIFY.load()

    def test_complete_ledger(self):
        """test_complete_ledger reproduces the published matrix sizes and exact traffic accounting."""
        self.assertEqual(76, len(self.rows))
        self.assertEqual(4, len(VERIFY.comparisons(self.rows, 'initial')))
        self.assertEqual(2, len(VERIFY.comparisons(self.rows, 'code-head')))

    def test_whole_cell_omission(self):
        """test_whole_cell_omission rejects removal of both variants instead of silently shrinking the matrix."""
        self.rows = [r for r in self.rows if not (r['study'] == 'initial' and r['profile'] == 'paced' and r['concurrency'] == 8)]
        with self.assertRaisesRegex(ValueError, 'missing planned trials'):
            VERIFY.validate(self.manifest, self.rows)

    def test_duplicate_trial(self):
        """test_duplicate_trial rejects an extra run even when its measurements are otherwise valid."""
        self.rows.append(copy.deepcopy(self.rows[0]))
        with self.assertRaisesRegex(ValueError, 'duplicate trial'):
            VERIFY.validate(self.manifest, self.rows)

    def test_pairwise_quota_change(self):
        """test_pairwise_quota_change detects billing differences despite positive request and quota totals."""
        next(r for r in self.rows if r['study'] == 'initial' and r['label'] == 'candidate')['billing_used_quota'] += 1
        with self.assertRaisesRegex(ValueError, 'paired billing mismatch'):
            VERIFY.validate(self.manifest, self.rows)

    def test_missing_qualification(self):
        """test_missing_qualification rejects absent cancellation evidence for an otherwise complete study."""
        del self.manifest['studies'][0]['qualification']['baseline']['cancellation']
        with self.assertRaisesRegex(ValueError, 'cancellation qualification failed'):
            VERIFY.validate(self.manifest, self.rows)

    def test_changed_binary(self):
        """test_changed_binary rejects mismatched binary provenance without trusting the study label."""
        self.manifest['studies'][0]['binaries']['baseline']['sha256'] = '0' * 64
        with self.assertRaisesRegex(ValueError, 'binary provenance mismatch'):
            VERIFY.validate(self.manifest, self.rows)

    def test_unaccounted_arrival(self):
        """test_unaccounted_arrival rejects a dropped request removed from the pressure ledger."""
        next(r for r in self.rows if r['dropped'])['dropped'] -= 1
        with self.assertRaisesRegex(ValueError, 'unaccounted traffic'):
            VERIFY.validate(self.manifest, self.rows)

    def test_corrupt_evidence_file(self):
        """test_corrupt_evidence_file rejects altered bytes before interpreting their measurements."""
        import json
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'manifest.json').write_text(json.dumps({'files': {'runs.csv': '0' * 64}}))
            (root / 'runs.csv').write_text('altered')
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                VERIFY.load(root)


if __name__ == '__main__':
    unittest.main()
