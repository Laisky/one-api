"""Correlate bounded SSE frame markers with client observations and complete runtime intervals."""
from __future__ import annotations
import argparse
from bisect import bisect_right
from collections import Counter, defaultdict
import hashlib
import json
import math
from pathlib import Path

from correlation_audit import check_inputs

PHASES = ('begin', 'flush_start', 'flush_end')


def require(condition: bool, message: str) -> None:
    """require rejects invalid evidence even when Python assertions are disabled."""
    if not condition:
        raise ValueError(message)


def distribution(values: list[float]) -> dict:
    """distribution summarizes the explicitly sampled population, retaining signed values where meaningful."""
    ordered = sorted(values)
    require(all(math.isfinite(v) for v in ordered), 'nonfinite measurement')
    if not ordered:
        return {'count': 0, 'p50': None, 'p95': None, 'max': None, 'min': None}
    return {'count': len(ordered), 'p50': ordered[(len(ordered) * 50 + 99)//100 - 1],
            'p95': ordered[(len(ordered) * 95 + 99)//100 - 1], 'max': ordered[-1], 'min': ordered[0]}


def overlap(intervals: list[tuple], starts: list[int], begin: int, end: int) -> float:
    """overlap sums intersections in an already nonoverlapping interval series, in milliseconds."""
    if end <= begin:
        return 0.0
    index = max(0, bisect_right(starts, begin) - 1)
    total = 0
    for left, right, *_ in intervals[index:]:
        if left >= end:
            break
        total += max(0, min(right, end) - max(left, begin))
    return total / 1e6


def union(intervals: list[tuple[int, int]]) -> list[tuple[int, int]]:
    """union merges overlapping GC ranges so concurrent scopes are never counted twice."""
    merged = []
    for begin, end in sorted(intervals):
        if merged and begin <= merged[-1][1]:
            merged[-1] = merged[-1][0], max(merged[-1][1], end)
        else:
            merged.append((begin, end))
    return merged


def runtime_intervals(events: list[dict]) -> tuple[dict, list, dict, dict]:
    """runtime_intervals reconstructs goroutine states and only fully observed GC ranges."""
    states, active, opened = defaultdict(list), {}, {}
    assists, pauses, excluded = defaultdict(list), [], Counter()
    last = -1
    for event in events:
        at = event['time']
        require(type(at) is int and at >= last, 'non-monotonic trace events')
        last = at
        kind = event['kind']
        if kind == 'StateTransition':
            gid = event['target_g']
            if gid in active:
                since, state, reason = active[gid]
                require(state == event['before'], 'inconsistent goroutine state transitions')
                states[gid].append((since, at, state, reason))
            active[gid] = at, event['after'], event['reason']
        elif kind in ('RangeBegin', 'RangeEnd', 'RangeActive'):
            key = event['name'], event['scope']
            if kind == 'RangeBegin':
                require(key not in opened, 'duplicate runtime range begin')
                opened[key] = at
            elif kind == 'RangeEnd':
                begin = opened.pop(key, None)
                if begin is None:
                    excluded['unmatched_range_ends'] += 1
                elif 'stop-the-world' in key[0]:
                    pauses.append((begin, at))
                elif 'mark assist' in key[0] and key[1].startswith('Goroutine('):
                    gid = int(key[1][len('Goroutine('):-1])
                    assists[gid].append((begin, at))
            else:
                excluded['active_at_trace_start'] += 1
    for gid, (since, state, reason) in active.items():
        states[gid].append((since, last, state, reason))
    excluded['unfinished_range_begins'] = len(opened)
    return dict(states), union(pauses), {gid: union(rows) for gid, rows in assists.items()}, dict(excluded)


def correlate(events: list[dict], requests: dict, chunks: int, absolute_clock: bool = False) -> dict:
    """correlate matches exact frame ordinals and decomposes observed gaps without asserting network or GC causality."""
    require(1 <= chunks <= 1024, 'unsupported chunk count')
    require(requests['failed'] == requests['dropped'] == 0 and requests['completed'] == requests['offered'], 'incomplete request delivery')
    samples = requests['samples']
    require(sorted(s['index'] for s in samples) == list(range(requests['offered'])), 'missing or duplicate request index')
    clients = {}
    for sample in samples:
        values = sample.get('observed_event_ns')
        selected = sample['index'] % 32 == 0
        require(not sample.get('error'), 'invalid sampled request')
        if selected:
            require(values is not None and len(values) == chunks + 4, 'data-event count mismatch')
            require(all(type(v) is int and v > 0 for v in values) and values == sorted(values), 'invalid client clock')
            clients[sample['index']] = values
        else:
            require(values is None, 'unregistered observation cohort')
    states, pauses, assists, excluded = runtime_intervals(events)
    frames = defaultdict(dict)
    for event in events:
        if event['kind'] != 'Log':
            continue
        request, frame, phase = event['request'], event['frame'], event['phase']
        require(request in clients and 0 <= frame < chunks+4 and phase in PHASES, 'invalid marker identity')
        require(phase not in frames[request, frame], 'duplicate frame marker')
        require(type(event['mono']) is int and event['mono'] > 0, 'invalid gateway monotonic clock')
        frames[request, frame][phase] = event
    require(frames, 'no gateway frame markers')
    complete, incomplete = {}, 0
    for key, markers in frames.items():
        if set(markers) != set(PHASES):
            incomplete += 1
            continue
        rows = [markers[p] for p in PHASES]
        require(len({r['g'] for r in rows}) == 1, 'frame crosses gateway goroutines')
        require([r['time'] for r in rows] == sorted(r['time'] for r in rows), 'phase trace order mismatch')
        require([r['mono'] for r in rows] == sorted(r['mono'] for r in rows), 'phase monotonic order mismatch')
        client = clients[key[0]][key[1]]
        if absolute_clock:
            require(client >= rows[0]['mono'], 'client event precedes gateway render entry')
        complete[key] = {**markers, 'client': client}
    pairs, observations = [], []
    state_groups = {}
    for gid, intervals in states.items():
        groups = defaultdict(list)
        for begin, end, state, reason in intervals:
            groups[state].append((begin, end))
            if state == 'Runnable' and 'Gosched' in reason:
                groups['GoschedRunnable'].append((begin, end))
        state_groups[gid] = {name: (rows, [i[0] for i in rows]) for name, rows in groups.items()}
    pause_starts = [x[0] for x in pauses]
    for (request, frame), current in sorted(complete.items()):
        if not 1 <= frame <= chunks:
            continue
        begin, flushing, end = [current[p] for p in PHASES]
        observations.append({'request': request, 'frame': frame,
            'render_to_flush_start_ms': (flushing['mono'] - begin['mono']) / 1e6,
            'flush_call_ms': (end['mono'] - flushing['mono']) / 1e6})
        if absolute_clock:
            observations[-1].update(begin_to_client_ms=(current['client'] - begin['mono'])/1e6,
                                    client_minus_flush_return_ms=(current['client'] - end['mono'])/1e6)
        previous = complete.get((request, frame-1))
        if previous is None or frame == 1:
            continue
        require(previous['flush_end']['g'] == begin['g'], 'adjacent frames changed gateway goroutine')
        before = previous['flush_end']
        left, right = before['time'], begin['time']
        require(right >= left and begin['mono'] >= before['mono'], 'adjacent frame order mismatch')
        alignment_error = ((right-left) - (begin['mono']-before['mono'])) / 1e6
        row = {'request': request, 'frame': frame, 'gateway_g': begin['g'],
            'client_gap_ms': (current['client'] - previous['client']) / 1e6,
            'gateway_flush_gap_ms': (end['mono'] - before['mono']) / 1e6,
            'between_render_calls_ms': (begin['mono'] - before['mono']) / 1e6,
            'downstream_lag_change_ms': ((current['client'] - previous['client']) - (end['mono'] - before['mono'])) / 1e6,
            'marker_interval_skew_ms': alignment_error,
            'state_alignment_usable': abs(alignment_error) <= 1.0}
        # All state/GC intersections use the trace clock and gateway marker bounds.
        # Client times are never converted to trace-clock absolute timestamps.
        if row['state_alignment_usable']:
            groups = state_groups.get(begin['g'], {})
            for state in ('Running', 'Runnable', 'Waiting', 'Syscall', 'GoschedRunnable'):
                series, starts = groups.get(state, ([], []))
                row[state.lower()+'_ms'] = overlap(series, starts, left, right)
            row['state_coverage_ms'] = sum(row[name+'_ms'] for name in ('running','runnable','waiting','syscall'))
            row['stw_overlap_ms'] = overlap(pauses, pause_starts, left, right)
            ranges = assists.get(begin['g'], [])
            row['mark_assist_overlap_ms'] = overlap(ranges, [x[0] for x in ranges], left, right)
        require(math.isclose(row['client_gap_ms'],row['gateway_flush_gap_ms']+row['downstream_lag_change_ms'],abs_tol=1e-6), 'gap decomposition mismatch')
        pairs.append(row)
    require(pairs, 'no complete adjacent content frames in trace')
    stalls = [row for row in pairs if row['client_gap_ms'] > 10]
    usable = [row for row in stalls if row['state_alignment_usable']]
    return {'scope': 'Diagnostic observations from a deterministic 1/32 request cohort, not unbiased latency or causal attribution.',
        'selected_requests': len(clients), 'matched_requests': len({k[0] for k in complete}),
        'complete_frames': len(complete), 'incomplete_boundary_frames': incomplete,
        'observed_content_frames': len(observations), 'adjacent_content_pairs': len(pairs),
        'alignment_excluded_pairs': sum(not row['state_alignment_usable'] for row in pairs),
        'excluded_runtime_ranges': excluded,
        'absolute_cross_process_verified': absolute_clock,
        'frame_metrics': {k: distribution([row[k] for row in observations]) for k in
            (['render_to_flush_start_ms','flush_call_ms'] + (['begin_to_client_ms','client_minus_flush_return_ms'] if absolute_clock else []))},
        'gap_metrics': {k: distribution([row[k] for row in pairs]) for k in (
            'client_gap_ms','gateway_flush_gap_ms','between_render_calls_ms','downstream_lag_change_ms','marker_interval_skew_ms')},
        'stalls_above_10ms': len(stalls), 'stalls_with_usable_state_alignment': len(usable),
        'stall_overlaps': {k: {'sum_ms':sum(row[k] for row in usable), **distribution([row[k] for row in usable])}
            for k in ('running_ms','runnable_ms','goschedrunnable_ms','waiting_ms','syscall_ms','stw_overlap_ms','mark_assist_overlap_ms')},
        'largest_observed_gaps': sorted(pairs,key=lambda row:row['client_gap_ms'],reverse=True)[:20],
        'pairs': pairs}


def audit_directory(directory: Path) -> dict:
    """audit_directory requires a qualified intact diagnostic and consistent clock identity before event correlation."""
    summary = json.loads((directory/'summary.json').read_text())
    require(summary['complete'] is True and summary['sustained_protocol_qualified'] is True, 'diagnostic did not qualify')
    requests = json.loads((directory/'requests.json').read_text())
    require({k:v for k,v in requests.items() if k!='samples'} == summary['requests'], 'request/summary mismatch')
    require(summary['usage']['requests'] == requests['completed'] and summary['usage']['used_quota']>0,'durable usage mismatch')
    snapshots = [json.loads(line) for line in (directory/'samples.jsonl').read_text().splitlines()]
    raw_checks = check_inputs(summary, requests, snapshots, (directory/'runtime.trace').read_bytes())
    domains = [row['clock_domain'] for row in snapshots]
    require(domains and all(d==domains[0] for d in domains), 'clock identity changed during workload')
    domain=domains[0]
    require(domain['clock']=='CLOCK_MONOTONIC' and len(set(domain['time_namespaces'].values()))==1,'clock namespaces differ')
    require(set(domain['time_namespaces'])=={'observer','gateway','mock','driver'},'incomplete clock evidence')
    metadata=json.loads((directory/'events.meta.json').read_text())
    require(metadata['exit_code']==0, 'trace decoder failed')
    require(metadata['trace_sha256']==hashlib.sha256((directory/'runtime.trace').read_bytes()).hexdigest(), 'trace identity mismatch')
    require(metadata['records_sha256']==hashlib.sha256((directory/'events.jsonl').read_bytes()).hexdigest(), 'decoded event identity mismatch')
    events=[json.loads(line) for line in (directory/'events.jsonl').read_text().splitlines()]
    absolute=domain.get('absolute_cross_process_verified') is True
    require(absolute == all(value is not None for value in domain['time_namespaces'].values()), 'inconsistent clock-verification flag')
    result=correlate(events,requests,summary['configuration']['chunks'],absolute)
    result['clock_domain']=domain
    result['raw_checks']=raw_checks
    result['inputs']={name:hashlib.sha256((directory/name).read_bytes()).hexdigest() for name in (
        'summary.json','requests.json','samples.jsonl','runtime.trace','events.jsonl')}
    return result


def main() -> None:
    """main audits one extracted correlation diagnostic and writes its full, explicitly scoped result."""
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory',type=Path)
    parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args()
    if args.output.exists():
        raise FileExistsError('refusing to overwrite correlation analysis')
    result=audit_directory(args.directory)
    args.output.write_text(json.dumps(result,indent=2,allow_nan=False)+'\n')
    print(json.dumps({k:v for k,v in result.items() if k!='pairs'},indent=2,allow_nan=False))


if __name__=='__main__':
    main()
