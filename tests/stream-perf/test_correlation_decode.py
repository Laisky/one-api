"""Decode known trace header shapes and reject failed, unbounded or overwritten observations."""
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

import correlation_decode as decode

MARKER='M=1 P=0 G=7 Log Time=123 Task=0 Category="oneapi.sse" Message="32/4/begin/120"'


class CorrelationDecodeTests(unittest.TestCase):
    """CorrelationDecodeTests test the real subprocess/filter boundary with controlled decoder output."""

    def test_exact_headers_not_attributes_or_stacks(self):
        """test_exact_headers_not_attributes_or_stacks distinguishes event type from strings inside unrelated debug fields."""
        self.assertEqual(decode.select(MARKER)['request'],32)
        self.assertIsNone(decode.select('  stack '+MARKER))
        self.assertIsNone(decode.select('M=1 P=0 G=7 Log Time=123 Task=0 Category="other" Message="RangeBegin"'))
        state=decode.select('M=1 P=0 G=7 StateTransition Time=123 GoID=8 Running->Runnable Reason="runtime.Gosched"')
        self.assertEqual(state['target_g'],8)
        self.assertEqual(state['reason'],'runtime.Gosched')
        self.assertIsNone(decode.select('M=1 P=0 G=7 StateTransition Time=123 ProcID=2 Running->Idle Reason=""'))

    def test_invalid_markers_never_become_measurements(self):
        """test_invalid_markers_never_become_measurements rejects out-of-range IDs and explicit measurement errors."""
        for line in (MARKER.replace('32/4','33/4'),MARKER.replace('32/4','8192/4'),
                     MARKER.replace('32/4','32/2048'),MARKER.replace('begin/120','begin/0'),
                     MARKER.replace('32/4/begin/120','bad'),
                     MARKER.replace('oneapi.sse','oneapi.sse.error')):
            with self.assertRaises(ValueError):decode.select(line)

    def run_decoder(self, body, limit=None, timeout=5):
        """run_decoder executes an actual test process and returns retained output or the expected exception."""
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);tool=root/'decoder';tool.write_text('#!'+sys.executable+'\n'+body);tool.chmod(0o700)
            trace=root/'trace';trace.write_bytes(b'test input')
            target=root/'events.jsonl'
            with patch.object(decode,'MAX_BYTES',limit or decode.MAX_BYTES):
                result=decode.decode(tool,trace,target,timeout=timeout)
            self.assertTrue(target.is_file());self.assertFalse(target.with_suffix('.partial').exists())
            self.assertTrue(target.with_suffix('.meta.json').is_file())
            with self.assertRaises(FileExistsError):decode.decode(tool,trace,target)
            return result

    def test_complete_decoder_is_published_once(self):
        """test_complete_decoder_is_published_once binds the tool, trace and filtered records without overwriting evidence."""
        result=self.run_decoder('print('+repr(MARKER)+')\n')
        self.assertEqual(result['counts']['Log'],1)
        self.assertEqual(result['exit_code'],0)

    def test_failed_silent_and_overflowed_decoders_fail(self):
        """test_failed_silent_and_overflowed_decoders_fail requires successful bounded output before publication."""
        for body,limit in [('import sys;sys.exit(2)',None),('import time;time.sleep(10)',None),('print('+repr(MARKER)+')',1)]:
            with self.subTest(body=body),self.assertRaises((RuntimeError,ValueError)):
                self.run_decoder(body,limit,timeout=1 if 'sleep' in body else 5)
