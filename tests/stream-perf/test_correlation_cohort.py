"""Preserve sampled state/GC intersections while bounding offline trace expansion."""
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

from correlation_analyze import correlate
import correlation_cohort as cohort
from correlation_decode import select


def headers():
    """headers supplies a full trace where relevant state starts before the first sampled frame."""
    rows = ['M=1 P=0 G=9 StateTransition Time=1 GoID=9 NotExist->Running Reason=""',
            'M=1 P=0 G=7 StateTransition Time=2 GoID=7 NotExist->Runnable Reason=""',
            'M=1 P=0 G=7 StateTransition Time=3 GoID=7 Runnable->Running Reason=""']
    for frame, start in ((1, 10), (2, 21)):
        for delta, phase in enumerate(('begin', 'flush_start', 'flush_end')):
            at = start + delta
            rows.append(f'M=1 P=0 G=7 Log Time={at} Task=0 Category="oneapi.sse" Message="0/{frame}/{phase}/{at*100}"')
        if frame == 1:
            rows.extend(['M=1 P=0 G=7 StateTransition Time=13 GoID=7 Running->Runnable Reason="runtime.Gosched"',
                         'M=1 P=0 G=8 RangeBegin Time=14 Name="stop-the-world (GC sweep termination)" Scope=Global',
                         'M=1 P=0 G=8 RangeEnd Time=15 Name="stop-the-world (GC sweep termination)" Scope=Global',
                         'M=1 P=0 G=9 StateTransition Time=16 GoID=9 Running->Waiting Reason="IO wait"',
                         'M=1 P=0 G=7 StateTransition Time=20 GoID=7 Runnable->Running Reason=""'])
    rows.append('M=1 P=0 G=9 StateTransition Time=35 GoID=9 Waiting->Runnable Reason="IO ready"')
    return rows


class CohortDecodeTests(unittest.TestCase):
    """CohortDecodeTests exercise real subprocesses, pass consistency, no-overwrite and analysis equivalence."""

    def setUp(self):
        """setUp creates private synthetic trace/tool artifacts and registers cleanup."""
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.trace = self.root / 'input.trace'
        self.trace.write_bytes(b'x')
        self.tool = self.root / 'decoder'
        self.output = self.root / 'events.jsonl'

    def tool_body(self, body):
        """tool_body installs a deterministic stand-in for the external pinned Go trace decoder."""
        self.tool.write_text('#!' + sys.executable + '\n' + body)
        self.tool.chmod(0o700)

    def test_correlations_match_full_state_decode(self):
        """test_correlations_match_full_state_decode retains pre-marker states, all GC and the real trace-end boundary."""
        lines = headers()
        self.tool_body('print(' + repr('\n'.join(lines)) + ')')
        result = cohort.decode_cohort(self.tool, self.trace, self.output)
        full = [event for line in lines if (event := select(line)) is not None]
        filtered = [json.loads(line) for line in self.output.read_text().splitlines()]
        requests = {'failed': 0, 'dropped': 0, 'completed': 1, 'offered': 1,
                    'samples': [{'index': 0, 'observed_event_ns': [100, 2000, 3200, 4000, 5000, 6000]}]}
        self.assertEqual(correlate(full, requests, 2), correlate(filtered, requests, 2))
        self.assertEqual(result['selected_gateway_goroutines'], [7])
        self.assertGreater(result['states_seen'], result['states_retained'])
        self.assertEqual(filtered[-1], {'kind': 'TraceBoundary', 'time': 35, 'g': -1})
        self.assertEqual(result['passes'][0], result['passes'][1])
        self.assertTrue(result['all_gc_ranges_retained'])
        with self.assertRaises(FileExistsError):
            cohort.decode_cohort(self.tool, self.trace, self.output)

    def test_marker_changes_between_passes_fail(self):
        """test_marker_changes_between_passes_fail rejects inconsistent decoder behavior without publishing success."""
        marker = headers()[3]
        self.tool_body('from pathlib import Path\np=Path(__file__).with_suffix(".counter")\n'
                       'n=int(p.read_text())+1 if p.exists() else 1\np.write_text(str(n))\n'
                       'print(' + repr(marker) + '.replace("/begin/1000", "/begin/"+str(1000+n)))')
        with self.assertRaisesRegex(ValueError, 'passes disagree'):
            cohort.decode_cohort(self.tool, self.trace, self.output)
        self.assertFalse(self.output.exists())
        self.assertTrue(self.output.with_suffix('.partial').exists())
        self.assertFalse(self.output.with_suffix('.meta.json').exists())

    def test_invalid_marker_is_not_skipped_during_discovery(self):
        """test_invalid_marker_is_not_skipped_during_discovery preserves observer error and cohort validation."""
        marker = headers()[3].replace('oneapi.sse', 'oneapi.sse.error')
        self.tool_body('print(' + repr(marker) + ')')
        with self.assertRaises(ValueError):
            cohort.decode_cohort(self.tool, self.trace, self.output)
        self.assertFalse(self.output.exists())

    def test_output_bound_is_not_raised(self):
        """test_output_bound_is_not_raised rejects excessive retained data at the same fixed size budget."""
        self.tool_body('print(' + repr(headers()[3]) + ')')
        with patch.object(cohort, 'MAX_BYTES', 10), self.assertRaisesRegex(ValueError, 'retained cohort output'):
            cohort.decode_cohort(self.tool, self.trace, self.output)
        self.assertFalse(self.output.exists())

    def test_silent_decoder_times_out(self):
        """test_silent_decoder_times_out enforces one end-to-end deadline without waiting for an output line."""
        self.tool_body('import time\ntime.sleep(10)')
        with self.assertRaises(RuntimeError):
            cohort.decode_cohort(self.tool, self.trace, self.output, timeout=1)
        self.assertFalse(self.output.exists())

    def test_missing_markers_fail(self):
        """test_missing_markers_fail requires a real observation cohort, not merely a successful child exit."""
        self.tool_body('print(' + repr(headers()[0]) + ')')
        with self.assertRaises(RuntimeError):
            cohort.decode_cohort(self.tool, self.trace, self.output)
        self.assertFalse(self.output.exists())


if __name__ == '__main__':
    unittest.main()
