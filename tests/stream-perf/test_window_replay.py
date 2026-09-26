"""Guard observation-origin provenance independently from the unchanged V2 admission rule."""
import copy
import unittest

from profile_window import stable_window
from window_replay import replay_window


def fixture(start=30.0):
    """fixture supplies slightly offset samples and an independently recorded original observation start."""
    rows = [{'elapsed': i + .0001, 'processes': {'gateway': {'cpu_seconds': i * 2.5}},
             'upstream': {'completed': i * 50, 'active': 32}, 'durable_requests': i * 50} for i in range(100)]
    summary = {'configuration': {'warmup': 30, 'seconds': 60, 'gateway_procs': 3},
               'allowance': {'effective_cores': 4}, 'window_started_elapsed': start,
               'window': stable_window(rows, 4, 3, start, 60)}
    return summary, rows


class WindowReplayTests(unittest.TestCase):
    """WindowReplayTests prevent a missing or different start from changing the saved acceptance outcome."""

    def test_explicit_origin_is_not_first_sample(self):
        """test_explicit_origin_is_not_first_sample exposes the legacy auditor's incorrect equality assumption."""
        summary, rows = fixture()
        self.assertNotEqual(summary['window'], stable_window(rows, 4, 3, summary['window']['intervals'][0]['start'], 60))
        result = replay_window(summary, rows)
        self.assertTrue(result['original_start_recorded'])
        self.assertEqual(result['reproduction_witnesses'], [{'convention': 'recorded', 'start': 30.0}])

    def test_legacy_nominal_reproduction_does_not_invent_origin(self):
        """test_legacy_nominal_reproduction_does_not_invent_origin keeps a reproduction witness distinct from recorded time."""
        summary, rows = fixture()
        del summary['window_started_elapsed']
        result = replay_window(summary, rows)
        self.assertFalse(result['original_start_recorded'])
        self.assertEqual(result['reproduction_witnesses'], [{'convention': 'nominal', 'start': 30.0}])
        self.assertFalse(result['qualification_changed'])

    def test_explicit_wrong_origin_never_falls_back(self):
        """test_explicit_wrong_origin_never_falls_back fails contradictory metadata even if a nominal replay would pass."""
        summary, rows = fixture()
        summary['window_started_elapsed'] = summary['window']['intervals'][0]['start']
        with self.assertRaisesRegex(ValueError, 'differs from raw counters'):
            replay_window(summary, rows)

    def test_all_fields_must_match(self):
        """test_all_fields_must_match rejects corruption in decisions, means, boundaries and half-window coverage."""
        for field in ('qualified', 'gateway_cores_mean', 'half_window_coverage_seconds', 'failure_reasons'):
            summary, rows = fixture()
            if field == 'qualified':
                summary['window'][field] = not summary['window'][field]
            elif field == 'gateway_cores_mean':
                summary['window'][field] += 1
            elif field == 'half_window_coverage_seconds':
                summary['window'][field][0] += .001
            else:
                summary['window'][field].append('invented')
            with self.subTest(field=field), self.assertRaises(ValueError):
                replay_window(summary, rows)

    def test_failed_record_is_preserved_even_when_nominal_would_pass(self):
        """test_failed_record_is_preserved_even_when_nominal_would_pass forbids selecting an alternate favorable window."""
        summary, rows = fixture(32.5)
        rows = rows[:91]
        summary['window'] = stable_window(rows, 4, 3, 32.5, 60)
        self.assertFalse(summary['window']['qualified'])
        self.assertTrue(stable_window(rows, 4, 3, 30.0, 60)['qualified'])
        before = copy.deepcopy(summary)
        result = replay_window(summary, rows)
        self.assertFalse(result['recorded_qualified'])
        self.assertEqual(summary, before)

    def test_invalid_or_late_origins_reject(self):
        """test_invalid_or_late_origins_reject disallows booleans, nonfinite numbers and arbitrary selected windows."""
        for origin in (True, '30', float('nan'), float('inf'), 29, 33):
            summary, rows = fixture()
            summary['window_started_elapsed'] = origin
            with self.subTest(origin=origin), self.assertRaises(ValueError):
                replay_window(summary, rows)


if __name__ == '__main__':
    unittest.main()
