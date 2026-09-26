"""Bind two-pass selected trace records to the full marker cohort and decoder identities."""
from __future__ import annotations
from collections import Counter
import hashlib
import json
from pathlib import Path

from correlation_audit import require
from correlation_decode import MAX_BYTES, keep


def audit_selection(output: Path, trace: Path) -> dict:
    """audit_selection verifies retained records, marker equality, selection scopes and both pass identities."""
    records = output.read_bytes()
    metadata = json.loads(output.with_suffix('.meta.json').read_text())
    trace_hash = hashlib.sha256(trace.read_bytes()).hexdigest()
    require(metadata['exit_code'] == 0 and metadata['trace_sha256'] == trace_hash and
            metadata['records_sha256'] == hashlib.sha256(records).hexdigest(), 'selected decoder identity mismatch')
    require(metadata['records_bytes'] == len(records) <= MAX_BYTES, 'selected decoder size mismatch')
    if metadata.get('selected_goroutines') is None:
        require(not metadata.get('marker_pass', False), 'marker-only data cannot provide state history')
        require(not output.with_suffix('.selection.json').exists(), 'inconsistent full-decoder selection metadata')
        return {'mode': 'whole trace', 'records_sha256': metadata['records_sha256']}
    selection = json.loads(output.with_suffix('.selection.json').read_text())
    marker_file = output.with_name(output.stem + '-markers.jsonl')
    marker_bytes = marker_file.read_bytes()
    marker_meta = json.loads(marker_file.with_suffix('.meta.json').read_text())
    require(selection['selected_pass'] == metadata and selection['marker_pass'] == marker_meta, 'pass metadata mismatch')
    require(marker_meta['exit_code'] == 0 and marker_meta['marker_pass'] is True and
            marker_meta['selected_goroutines'] is None and metadata['marker_pass'] is False, 'invalid selection phases')
    require(marker_meta['trace_sha256'] == trace_hash and marker_meta['tool_sha256'] == metadata['tool_sha256'],
            'trace or decoder changed between passes')
    require(marker_meta['records_sha256'] == hashlib.sha256(marker_bytes).hexdigest() and
            marker_meta['records_bytes'] == len(marker_bytes) <= MAX_BYTES, 'marker-pass identity or size mismatch')
    markers = [json.loads(line) for line in marker_bytes.splitlines()]
    require(markers and all(event['kind'] == 'Log' for event in markers), 'first pass contains non-marker records')
    selected = {event['g'] for event in markers}
    require(len(selected) <= 8192 and all(type(g) is int and g >= 0 for g in selected), 'invalid selected identity')
    require(metadata['selected_goroutines'] == sorted(selected), 'goroutine selection differs from full marker cohort')
    require(marker_meta['counts'] == {'Log': len(markers)}, 'incorrect marker counts')
    matched, counts = 0, Counter()
    for line in records.splitlines():
        event = json.loads(line)
        counts[event['kind']] += 1
        require(keep(event, False, selected), 'unselected runtime record retained')
        if event['kind'] == 'Log':
            require(matched < len(markers) and event == markers[matched], 'marker changed or reordered across passes')
            matched += 1
    require(matched == len(markers) and dict(counts) == metadata['counts'], 'missing marker or incorrect retained counts')
    return {'mode': 'two-pass selected goroutines', 'selected_goroutines': sorted(selected),
            'marker_count': len(markers), 'records_sha256': metadata['records_sha256'],
            'marker_records_sha256': marker_meta['records_sha256'], 'trace_sha256': trace_hash,
            'tool_sha256': metadata['tool_sha256'],
            'boundary': 'Full captured histories for the marker-selected goroutines, not all runtime goroutines.'}
