"""Audit two-pass metadata and marker-preserving selection independently of the Go decoder."""
from collections import Counter
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from correlation_selection import audit_selection


def fixture(root, change=None):
    """fixture binds small synthetic records so deliberate semantic errors cannot hide behind valid checksums."""
    trace = root / 'runtime.trace'
    trace.write_bytes(b'fixed test trace')
    output = root / 'events.jsonl'
    marker = {'kind': 'Log', 'g': 7, 'time': 30, 'request': 0, 'frame': 1, 'phase': 'begin', 'mono': 3}
    markers = [marker]
    rows = [{'kind': 'StateTransition', 'g': 9, 'target_g': 7, 'time': 1,
             'before': 'Undetermined', 'after': 'Runnable', 'reason': ''}, dict(marker)]
    if change:
        change(markers, rows)
    def save(path, events, marker_pass):
        """save writes canonical test data and its independently calculated metadata."""
        data = ''.join(json.dumps(event) + '\n' for event in events).encode()
        path.write_bytes(data)
        metadata = {'exit_code': 0, 'trace_sha256': hashlib.sha256(trace.read_bytes()).hexdigest(),
                    'tool_sha256': 'fixed decoder', 'records_sha256': hashlib.sha256(data).hexdigest(),
                    'records_bytes': len(data), 'counts': dict(Counter(e['kind'] for e in events)),
                    'marker_pass': marker_pass, 'selected_goroutines': None if marker_pass else [7]}
        path.with_suffix('.meta.json').write_text(json.dumps(metadata))
        return metadata
    first = save(root / 'events-markers.jsonl', markers, True)
    second = save(output, rows, False)
    output.with_suffix('.selection.json').write_text(json.dumps({'marker_pass': first, 'selected_pass': second}))
    return output, trace


class SelectionAuditTests(unittest.TestCase):
    """SelectionAuditTests reject metadata drift, missing markers and unrelated state histories."""

    def test_complete_selected_history(self):
        """test_complete_selected_history accepts pre-marker state and exact two-pass identity."""
        with tempfile.TemporaryDirectory() as directory:
            result = audit_selection(*fixture(Path(directory)))
            self.assertEqual(result['selected_goroutines'], [7])
            self.assertEqual(result['marker_count'], 1)

    def test_rehashed_semantic_corruption(self):
        """test_rehashed_semantic_corruption rejects omissions and unrelated records despite matching file hashes."""
        changes = [lambda m, r: r.pop(), lambda m, r: r[-1].update(mono=4),
                   lambda m, r: r[0].update(target_g=8), lambda m, r: m[0].update(g=8),
                   lambda m, r: m.append({'kind': 'StateTransition', 'g': 7}),
                   lambda m, r: r.append({'kind': 'RangeBegin', 'g': 8, 'time': 40,
                                          'name': 'GC mark assist', 'scope': 'Goroutine(8)'})]
        for change in changes:
            with self.subTest(change=change), tempfile.TemporaryDirectory() as directory:
                with self.assertRaises(ValueError):
                    audit_selection(*fixture(Path(directory), change))

    def test_file_and_manifest_drift(self):
        """test_file_and_manifest_drift rejects changed raw files and mismatched pass records."""
        for filename in ('runtime.trace', 'events.jsonl', 'events-markers.jsonl', 'events.selection.json'):
            with self.subTest(file=filename), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                output, trace = fixture(root)
                path = root / filename
                if filename.endswith('selection.json'):
                    metadata = json.loads(path.read_text())
                    metadata['selected_pass']['tool_sha256'] = 'another decoder'
                    path.write_text(json.dumps(metadata))
                else:
                    path.write_bytes(path.read_bytes() + b'corruption')
                with self.assertRaises(ValueError):
                    audit_selection(output, trace)

    def test_whole_trace_is_not_marker_only(self):
        """test_whole_trace_is_not_marker_only preserves full-decoder compatibility without accepting a marker-only history."""
        with tempfile.TemporaryDirectory() as directory:
            output, trace = fixture(Path(directory))
            metadata_path = output.with_suffix('.meta.json')
            metadata = json.loads(metadata_path.read_text())
            metadata['selected_goroutines'] = None
            metadata_path.write_text(json.dumps(metadata))
            output.with_suffix('.selection.json').unlink()
            self.assertEqual(audit_selection(output, trace)['mode'], 'whole trace')
            metadata['marker_pass'] = True
            metadata_path.write_text(json.dumps(metadata))
            with self.assertRaises(ValueError):
                audit_selection(output, trace)
