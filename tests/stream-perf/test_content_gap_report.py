"""Behavior tests keep stream continuity visible without fabricating missing legacy telemetry."""
import math
import unittest

import report
from test_report import fixture


class ContentGapReportTests(unittest.TestCase):
    """ContentGapReportTests cover regressions, zero gaps, missing fields and invalid evidence."""

    def test_reports_continuity_regression_even_with_faster_throughput(self):
        """test_reports_continuity_regression_even_with_faster_throughput prevents a throughput-only verdict."""
        study = fixture()
        for trial in study['trials']:
            trial['max_inter_content_gap_ms'] = {'p95': 10 if trial['label'] == 'baseline' else 30}
        cells = report.compare(study, 3)
        gap = cells[0]['metrics']['max_inter_content_gap_p95_ms']
        self.assertEqual(gap['paired_change_pct_median'], 200)
        self.assertEqual(gap['paired_change_abs_median'], 20)
        text = report.markdown(cells)
        self.assertIn('Streaming continuity', text)
        self.assertIn('+200.00%', text)
        self.assertIn('not a pooled token-gap percentile', text)

    def test_legacy_is_explicitly_unmeasured(self):
        """test_legacy_is_explicitly_unmeasured keeps historical records usable without inventing zero gaps."""
        cells = report.compare(fixture(), 3)
        self.assertNotIn('max_inter_content_gap_p95_ms', cells[0]['metrics'])
        self.assertIn('not captured in this legacy study', report.markdown(cells))

    def test_mixed_telemetry_is_rejected(self):
        """test_mixed_telemetry_is_rejected prevents omission of an unfavorable variant or repetition."""
        for missing in range(6):
            with self.subTest(missing=missing):
                study = fixture()
                for index, trial in enumerate(study['trials']):
                    if index != missing:
                        trial['max_inter_content_gap_ms'] = {'p95': 5}
                with self.assertRaisesRegex(ValueError, 'every trial'):
                    report.compare(study, 3)

    def test_zero_gap_keeps_absolute_comparison(self):
        """test_zero_gap_keeps_absolute_comparison handles single-content streams without dividing by zero."""
        study = fixture()
        for trial in study['trials']:
            trial['max_inter_content_gap_ms'] = {'p95': 0 if trial['label'] == 'baseline' else 2}
        cells = report.compare(study, 3)
        gap = cells[0]['metrics']['max_inter_content_gap_p95_ms']
        self.assertIsNone(gap['paired_change_pct_median'])
        self.assertIsNone(gap['paired_change_pct_min'])
        self.assertIsNone(gap['paired_change_pct_max'])
        self.assertEqual(gap['paired_change_abs_median'], 2)
        self.assertIn('undefined (zero baseline)', report.markdown(cells))

    def test_invalid_gap_values_are_rejected(self):
        """test_invalid_gap_values_are_rejected excludes booleans, negative durations and nonfinite numbers."""
        for invalid in (True, False, -1, math.nan, math.inf, -math.inf, '1', None):
            with self.subTest(value=invalid):
                study = fixture()
                for trial in study['trials']:
                    trial['max_inter_content_gap_ms'] = {'p95': 1}
                study['trials'][-1]['max_inter_content_gap_ms']['p95'] = invalid
                with self.assertRaises(ValueError):
                    report.compare(study, 3)
