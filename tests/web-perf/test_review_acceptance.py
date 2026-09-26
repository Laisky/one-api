"""Regression controls for complete freshness evidence and ordered index identity."""
import itertools
import json
from pathlib import Path
import tempfile
import unittest

import report
from test_contracts import evidence

FRESHNESS = ('committed_insert', 'legacy_rows_and_count', 'all_dashboard_aggregates')
COLUMNS = ['user_id', 'created_at', 'id']


def complete_evidence() -> dict:
    """complete_evidence supplies the full successful contract without depending on the check under test."""
    summary = evidence()
    for label in ('baseline', 'candidate'):
        summary['freshness'][label] = dict.fromkeys(FRESHNESS, True)
        summary['schema'][label]['index_columns'] = COLUMNS.copy() if label == 'candidate' else []
    return summary


class ReviewAcceptanceTests(unittest.TestCase):
    """ReviewAcceptanceTests prevent stale responses or a wrongly ordered index from earning acceptance."""

    def test_complete_contract_accepts_in_both_modes(self):
        """test_complete_contract_accepts_in_both_modes keeps a positive full-matrix control for both API modes."""
        for strict in (False, True):
            result = report.compare(complete_evidence(), strict=strict)
            self.assertTrue(result['accepted'])
            self.assertEqual(result['completed'], 51200)
            self.assertFalse(result['raw_samples_verified'])

    def test_each_freshness_flag_requires_literal_true(self):
        """test_each_freshness_flag_requires_literal_true rejects absent, false and truthy nonboolean evidence per variant."""
        for strict, label, field, missing, invalid in itertools.product(
                (False, True), ('baseline', 'candidate'), FRESHNESS, (False, True), (False, None, 0, 1, 'true')):
            if missing and invalid is not False:
                continue
            with self.subTest(strict=strict, label=label, field=field, missing=missing, value=invalid):
                summary = complete_evidence()
                if missing:
                    summary['freshness'][label].pop(field)
                else:
                    summary['freshness'][label][field] = invalid
                with self.assertRaisesRegex(ValueError, 'freshness'):
                    report.compare(summary, strict=strict)

    def test_missing_variant_freshness_rejects(self):
        """test_missing_variant_freshness_rejects requires evidence from both independently qualified gateways."""
        for label in ('baseline', 'candidate'):
            summary = complete_evidence()
            summary['freshness'].pop(label)
            with self.assertRaisesRegex(ValueError, 'freshness'):
                report.compare(summary)

    def test_index_name_does_not_substitute_for_ordered_columns(self):
        """test_index_name_does_not_substitute_for_ordered_columns rejects all permutations and malformed column lists."""
        invalid = [list(p) for p in itertools.permutations(COLUMNS) if list(p) != COLUMNS]
        invalid += [[], ['user_id'], COLUMNS[:2], COLUMNS + ['type'], ['user_id', 'created_at', 'created_at'],
                    None, 'user_id,created_at,id', True]
        for strict in (False, True):
            for columns in invalid:
                with self.subTest(strict=strict, columns=columns):
                    summary = complete_evidence()
                    summary['schema']['candidate']['index_columns'] = columns
                    with self.assertRaisesRegex(ValueError, 'index.*column'):
                        report.compare(summary, strict=strict)
            with self.subTest(strict=strict, columns='missing'):
                summary = complete_evidence()
                summary['schema']['candidate'].pop('index_columns')
                with self.assertRaisesRegex(ValueError, 'index.*column'):
                    report.compare(summary, strict=strict)

    def test_raw_audit_rejects_bad_contract_before_reading_samples(self):
        """test_raw_audit_rejects_bad_contract_before_reading_samples exercises the real directory audit entry point."""
        for wrong_index in (False, True):
            summary = complete_evidence()
            if wrong_index:
                summary['schema']['candidate']['index_columns'] = COLUMNS[::-1]
            else:
                summary['freshness']['baseline']['legacy_rows_and_count'] = False
            with tempfile.TemporaryDirectory() as name:
                root = Path(name)
                (root / 'summary.json').write_text(json.dumps(summary))
                # There are deliberately no sample files. Contract rejection must occur first.
                with self.subTest(wrong_index=wrong_index):
                    try:
                        with self.assertRaises(ValueError):
                            report.audit(root)
                    except FileNotFoundError:
                        self.fail('audit reached request records before rejecting invalid contract')


if __name__ == '__main__':
    unittest.main()
