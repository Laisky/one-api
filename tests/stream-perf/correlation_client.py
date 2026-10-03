"""Match client and gateway frame intervals without assuming a common absolute clock epoch."""
from __future__ import annotations
import argparse
from collections import defaultdict
import hashlib
import json
import math
from pathlib import Path

from correlation_analyze import audit_directory, correlate, distribution, overlap, require, runtime_intervals
from correlation_selection import audit_selection


def combine(gateway_events: list[dict], client_events: list[dict], requests: dict, chunks: int) -> dict:
    """combine attributes each matched interval only to states on its own process's trace clock."""
    gateway = correlate(gateway_events, requests, chunks, False)
    states, pauses, assists, excluded = runtime_intervals(client_events)
    samples = {s['index']: s['observed_event_ns'] for s in requests['samples'] if 'observed_event_ns' in s}
    frames = {}
    for event in client_events:
        if event['kind'] != 'Log':
            continue
        require(event.get('phase') == 'observe', 'non-client marker in client trace')
        request, frame, gid = event['request'], event['frame'], event['g']
        require(type(request) is int and request in samples and type(frame) is int and 0 <= frame < chunks + 4, 'invalid client frame identity')
        require(type(gid) is int and gid >= 0, 'invalid client goroutine')
        require(type(event['mono']) is int and event['mono'] == samples[request][frame], 'client marker/sample clock disagreement')
        require((request, frame) not in frames, 'duplicate client frame marker')
        frames[request, frame] = event
    require(frames, 'no sampled client trace markers')
    groups = {}
    for gid, intervals in states.items():
        grouped = defaultdict(list)
        for begin, end, state, reason in intervals:
            grouped[state].append((begin, end))
            if state == 'Waiting' and 'network' in reason.lower():
                grouped['WaitingNetwork'].append((begin, end))
        groups[gid] = {k: (v, [x[0] for x in v]) for k, v in grouped.items()}
    rows, boundary = [], 0
    for previous in gateway['pairs']:
        key = previous['request'], previous['frame']
        left, right = frames.get((key[0], key[1] - 1)), frames.get(key)
        if left is None or right is None:
            boundary += 1
            continue
        require(left['g'] == right['g'], 'client request moved between goroutines')
        start, finish = left['time'], right['time']
        require(finish >= start and right['mono'] >= left['mono'], 'client frame order mismatch')
        skew = ((finish - start) - (right['mono'] - left['mono'])) / 1e6
        row = {**previous, 'client_g': right['g'], 'client_marker_interval_skew_ms': skew,
               'client_state_alignment_usable': abs(skew) <= 1,
               'client_trace_interval_ms': (finish - start) / 1e6}
        require(math.isclose(row['client_gap_ms'], (right['mono']-left['mono'])/1e6, abs_tol=1e-9), 'client gap mismatch')
        if row['client_state_alignment_usable']:
            for state in ('Running', 'Runnable', 'Waiting', 'Syscall', 'WaitingNetwork'):
                series, starts = groups.get(right['g'], {}).get(state, ([], []))
                row['client_' + state.lower() + '_ms'] = overlap(series, starts, start, finish)
            row['client_state_coverage_ms'] = sum(row['client_' + state + '_ms'] for state in ('running','runnable','waiting','syscall'))
            row['client_state_coverage_complete'] = math.isclose(row['client_state_coverage_ms'], row['client_trace_interval_ms'], abs_tol=1e-6)
            row['client_stw_overlap_ms'] = overlap(pauses, [x[0] for x in pauses], start, finish)
            ranges = assists.get(right['g'], [])
            row['client_mark_assist_overlap_ms'] = overlap(ranges, [x[0] for x in ranges], start, finish)
        rows.append(row)
    require(rows, 'no shared adjacent content pair between traces')
    stalls = [r for r in rows if r['client_gap_ms'] > 10]
    usable = [r for r in stalls if r['state_alignment_usable'] and r['client_state_alignment_usable'] and r['client_state_coverage_complete']]
    excess = [r for r in usable if r['downstream_lag_change_ms'] > 5]
    def categories(population):
        """categories counts separately scoped overlaps, never adding gateway and client durations."""
        return {'pairs': len(population),
                'client_runnable_at_least_80pct': sum(r['client_runnable_ms'] >= .8*r['client_trace_interval_ms'] for r in population),
                'client_waiting_at_least_80pct': sum(r['client_waiting_ms'] >= .8*r['client_trace_interval_ms'] for r in population),
                'client_network_waiting_at_least_80pct': sum(r['client_waitingnetwork_ms'] >= .8*r['client_trace_interval_ms'] for r in population),
                'gateway_runnable_at_least_80pct': sum(r['between_render_calls_ms'] > 0 and r['runnable_ms'] >= .8*r['between_render_calls_ms'] for r in population)}
    return {'scope': 'Two instrumented processes; per-process intervals only, not packet timing, causal latency or an unprofiled comparison.',
            'absolute_cross_process_comparison': False, 'gateway_adjacent_pairs': len(gateway['pairs']),
            'matched_adjacent_pairs': len(rows), 'unmatched_trace_boundary_pairs': boundary,
            'matched_requests': len({r['request'] for r in rows}), 'client_frame_markers': len(frames),
            'client_excluded_runtime_ranges': excluded,
            'client_alignment_excluded_pairs': sum(not r['client_state_alignment_usable'] for r in rows),
            'stalls_above_10ms': len(stalls), 'usable_stalls': categories(usable),
            'client_gap_exceeds_gateway_gap_by_5ms': categories(excess),
            'client_gap_ms': distribution([r['client_gap_ms'] for r in rows]),
            'largest_gaps': sorted(rows, key=lambda r: r['client_gap_ms'], reverse=True)[:20], 'pairs': rows}


def audit(directory: Path) -> dict:
    """audit requires qualified two-trace capture and binds marker analysis to raw request and decoder identities."""
    selection = {role: audit_selection(directory / records, directory / trace)
                 for role, records, trace in [('gateway', 'events.jsonl', 'runtime.trace'),
                                               ('client', 'client-events.jsonl', 'client.trace')]}
    prior = audit_directory(directory)
    summary = json.loads((directory/'summary.json').read_text())
    require(summary['configuration'].get('client_trace') is True and summary.get('client_trace_window_valid') is True, 'client capture not qualified')
    captured = summary['client_trace_capture']
    data = (directory/'client.trace').read_bytes()
    require(captured['process_role'] == 'driver' and captured['file'] == 'client.trace' and captured['bytes'] == len(data) and captured['sha256'] == hashlib.sha256(data).hexdigest(), 'client capture identity mismatch')
    require(summary['configuration']['warmup'] <= captured['started_elapsed'] < captured['finished_elapsed'] <= summary['configuration']['warmup'] + summary['configuration']['seconds'], 'client trace outside window')
    metadata = json.loads((directory/'client-events.meta.json').read_text())
    records = (directory/'client-events.jsonl').read_bytes()
    require(metadata['exit_code'] == 0 and metadata['trace_sha256'] == hashlib.sha256(data).hexdigest() and metadata['records_sha256'] == hashlib.sha256(records).hexdigest(), 'client decoder identity mismatch')
    gateway = [json.loads(line) for line in (directory/'events.jsonl').read_text().splitlines()]
    client = [json.loads(line) for line in records.splitlines()]
    requests = json.loads((directory/'requests.json').read_text())
    result = combine(gateway, client, requests, summary['configuration']['chunks'])
    result.update(decoder_selection=selection, clock_domain=prior['clock_domain'], raw_checks=prior['raw_checks'],
                  inputs={n:hashlib.sha256((directory/n).read_bytes()).hexdigest() for n in ('summary.json','requests.json','events.jsonl','client-events.jsonl','runtime.trace','client.trace','samples.jsonl')})
    return result


def main() -> None:
    """main audits one paired-trace directory and exclusively publishes the complete scoped analysis."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = audit(args.directory)
    with args.output.open('x') as destination:
        json.dump(result,destination,indent=2,allow_nan=False);destination.write('\n')
    print(json.dumps({k:v for k,v in result.items() if k not in ('pairs','largest_gaps')},indent=2))


if __name__ == '__main__':
    main()
