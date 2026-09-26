"""Regression guards for stable high-load evidence, not performance claims."""
import math
import unittest

import profile_support


def fixture_samples():
    """fixture_samples returns a complete busy window with independently monotonic progress and CPU."""
    return [{'elapsed': float(i), 'processes': {'gateway': {'cpu_seconds': i * 2.8}},
             'upstream': {'completed': i * 50, 'active': 8}, 'durable_requests': i * 50}
            for i in range(92)]


def evaluate(rows, **options):
    """evaluate applies the public fixed 30-second warm-up/60-second observation contract."""
    return profile_support.stable_window(rows, 4, 3, 30, 60, **options)


class ProfileWindowTests(unittest.TestCase):
    """ProfileWindowTests ensure busy CPU cannot conceal a stalled, drifting or corrupt workload."""

    def test_busy_without_completed_requests_is_not_stable(self):
        """test_busy_without_completed_requests_is_not_stable rejects CPU-only false qualification."""
        rows = fixture_samples()
        for row in rows:
            row['upstream']['completed'] = row['durable_requests'] = 0
        self.assertFalse(evaluate(rows)['qualified'])

    def test_rate_drift_rejects_even_when_cpu_stays_busy(self):
        """test_rate_drift_rejects_even_when_cpu_stays_busy rejects an unexplained 50 percent slowdown."""
        rows = fixture_samples()
        for row in rows[61:]:
            row['upstream']['completed'] = 3000 + (int(row['elapsed']) - 60) * 25
            row['durable_requests'] = row['upstream']['completed']
        self.assertFalse(evaluate(rows)['qualified'])

    def test_completed_and_durable_counters_may_not_roll_back(self):
        """test_completed_and_durable_counters_may_not_roll_back rejects counter corruption in either source."""
        for field in ('completed', 'durable_requests'):
            rows = fixture_samples()
            if field == 'completed':
                rows[45]['upstream'][field] = 0
            else:
                rows[45][field] = 0
            with self.subTest(field=field), self.assertRaises(ValueError):
                evaluate(rows)

    def test_corrupt_counts_are_not_zero_or_real_progress(self):
        """test_corrupt_counts_are_not_zero_or_real_progress rejects negative, nonfinite and boolean counts."""
        for bad in (-1, math.nan, math.inf, True, '4'):
            rows = fixture_samples()
            rows[45]['upstream']['completed'] = bad
            with self.subTest(value=bad), self.assertRaises(ValueError):
                evaluate(rows)

    def test_valid_window_keeps_time_and_progress_attribution(self):
        """test_valid_window_keeps_time_and_progress_attribution reports the exact fixed-rate fixture."""
        result = evaluate(fixture_samples())
        self.assertTrue(result['qualified'])
        self.assertEqual(result['rule_version'], 2)
        self.assertAlmostEqual(result['gateway_cores_mean'], 2.8)
        self.assertEqual(result['half_window_upstream_rps'], [50.0, 50.0])
        self.assertEqual(result['upstream_rate_drift'], 0)
        self.assertEqual(result['failure_reasons'], [])

    def test_all_counters_and_times_are_monotonic(self):
        """test_all_counters_and_times_are_monotonic rejects process/time rollback instead of dropping bad intervals."""
        for mutate in (lambda r: r[45].update(elapsed=r[44]['elapsed']),
                       lambda r: r[45]['processes']['gateway'].update(cpu_seconds=0),
                       lambda r: r[45]['upstream'].update(active=-1)):
            rows = fixture_samples()
            mutate(rows)
            with self.assertRaises(ValueError):
                evaluate(rows)

    def test_partial_half_and_long_sampling_gap_fail(self):
        """test_partial_half_and_long_sampling_gap_fail requires both halves to represent the observation."""
        for rows in (fixture_samples()[:71], fixture_samples()[:45] + fixture_samples()[49:]):
            with self.subTest(length=len(rows)):
                self.assertFalse(evaluate(rows)['qualified'])

    def test_gate_parameters_are_validated(self):
        """test_gate_parameters_are_validated rejects nonsensical fractions and timing windows."""
        for options in ({'minimum': math.nan}, {'fraction': 0}, {'fraction': 1.1},
                        {'minimum': -1}, {'max_rate_drift': -1}, {'max_rate_drift': math.inf}):
            with self.subTest(options=options), self.assertRaises(ValueError):
                evaluate(fixture_samples(), **options)
        for start, seconds in ((-1, 60), (30, 0), (math.nan, 60), (30, math.inf)):
            with self.assertRaises(ValueError):
                profile_support.stable_window(fixture_samples(), 4, 3, start, seconds)

    def test_halves_use_time_instead_of_number_of_samples(self):
        """test_halves_use_time_instead_of_number_of_samples is invariant to extra sampling in the first half."""
        rows = fixture_samples()
        extra = []
        for row in rows:
            if 30 <= row['elapsed'] < 60:
                value = row['elapsed'] + .5
                extra.append({'elapsed': value, 'processes': {'gateway': {'cpu_seconds': value * 2.8}},
                              'upstream': {'completed': int(value * 50), 'active': 8},
                              'durable_requests': int(value * 50)})
        result = evaluate(sorted(rows + extra, key=lambda row: row['elapsed']))
        self.assertTrue(result['qualified'])
        self.assertEqual(result['half_window_upstream_rps'], [50.0, 50.0])
        self.assertAlmostEqual(result['gateway_cores_mean'], 2.8)
        self.assertAlmostEqual(result['coverage_seconds'], 60)

    def test_duration_weighted_cpu_rejects_dense_high_samples(self):
        """test_duration_weighted_cpu_rejects_dense_high_samples cannot be fooled by high-rate sampling of busy periods."""
        rows = []
        elapsed = cpu = 0.0
        count = 0
        rows.append({'elapsed': elapsed, 'processes': {'gateway': {'cpu_seconds': cpu}},
                     'upstream': {'completed': count, 'active': 8}, 'durable_requests': count})
        # Every second has 90% busy observations by count, but only 45% busy time.
        for second in range(91):
            for step in range(10):
                duration, cores = (.05, 3.0) if step < 9 else (.55, .5)
                elapsed = second + (step + 1) * .05 if step < 9 else float(second + 1)
                cpu += duration * cores
                count = round(elapsed * 100)
                rows.append({'elapsed': elapsed, 'processes': {'gateway': {'cpu_seconds': cpu}},
                             'upstream': {'completed': count, 'active': 8}, 'durable_requests': count})
        result = evaluate(rows)
        self.assertFalse(result['qualified'])
        self.assertAlmostEqual(result['fraction_at_or_above_target'], .45, places=6)
        self.assertAlmostEqual(result['gateway_cores_mean'], 1.625, places=6)
