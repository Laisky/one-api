"""Prove bounded two-pass decoding retains state before markers without retaining unrelated goroutines."""
import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
import correlation_decode as decode


class SelectionTests(unittest.TestCase):
    """SelectionTests prevent trace-size workarounds from fabricating missing scheduler history."""

    def test_two_pass_selection_keeps_premarker_history(self):
        """test_two_pass_selection_keeps_premarker_history excludes unrelated volume without raising the output cap."""
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); tool=root/'decoder'; trace=root/'trace'; trace.write_bytes(b'trace identity')
            tool.write_text('#!'+sys.executable+'\n'+'''
print('M=1 P=0 G=7 StateTransition Time=1 GoID=7 Undetermined->Runnable Reason=""')
for i in range(2000):
    print(f'M=1 P=0 G=8 StateTransition Time={i+2} GoID=8 Running->Runnable Reason="runtime.Gosched"')
print('M=1 P=0 G=7 StateTransition Time=3000 GoID=7 Runnable->Running Reason=""')
print('M=1 P=0 G=7 Log Time=3001 Category="oneapi.sse.client" Message="32/2/observe/900"')
print('M=1 P=0 G=99 RangeBegin Time=3002 Name="stop-the-world (GC)" Scope=Goroutine(99)')
print('M=1 P=0 G=99 RangeEnd Time=3003 Name="stop-the-world (GC)" Scope=Goroutine(99)')
''');tool.chmod(0o700)
            with patch.object(decode,'MAX_BYTES',2048):
                with self.assertRaises(ValueError):decode.decode(tool,trace,root/'full.jsonl')
                result=decode.decode_selected(tool,trace,root/'selected.jsonl')
            rows=[json.loads(line) for line in (root/'selected.jsonl').read_text().splitlines()]
            self.assertEqual(rows[0]['time'],1)
            self.assertEqual(rows[0]['target_g'],7)
            self.assertEqual(len(rows),5)
            self.assertEqual(result['selected_pass']['selected_goroutines'],[7])
            self.assertEqual(result['marker_pass']['counts'],{'Log':1})
            self.assertEqual(result['marker_pass']['trace_sha256'],result['selected_pass']['trace_sha256'])
            self.assertEqual(result['selected_pass']['records_sha256'],hashlib.sha256((root/'selected.jsonl').read_bytes()).hexdigest())
            self.assertTrue((root/'full.partial').exists())
            with self.assertRaises(FileExistsError):decode.decode_selected(tool,trace,root/'selected.jsonl')

    def test_gc_assists_are_scoped_but_global_pauses_are_not(self):
        """test_gc_assists_are_scoped_but_global_pauses_are_not preserves the scopes needed for local overlap analysis."""
        for name,scope,expected in [('GC mark assist','Goroutine(7)',True),('GC mark assist','Goroutine(8)',False),('stop-the-world (GC)','Goroutine(8)',True)]:
            event={'kind':'RangeBegin','name':name,'scope':scope}
            self.assertEqual(decode.keep(event,False,{7}),expected)
            self.assertFalse(decode.keep(event,True,None))

    def test_invalid_selection_never_starts_decoder(self):
        """test_invalid_selection_never_starts_decoder bounds selection cardinality and numeric identities."""
        with patch.object(decode.subprocess,'Popen') as process:
            for selected in (set(),{-1},{True},set(range(8193))):
                with self.assertRaises(ValueError):decode.decode(Path('/unused'),Path('/unused'),Path('/unused'),selected_gids=selected)
            process.assert_not_called()
