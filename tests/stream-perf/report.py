#!/usr/bin/env python3
"""Compare paired run-level E2E observations without treating requests as independent experiments."""
from __future__ import annotations
import argparse
from collections import defaultdict
import json
import math
from pathlib import Path
import statistics

METRICS = {
    'successful_rps': ('successful_rps',),
    'ttft_p95_ms': ('ttft_ms', 'p95'),
    'completion_p95_ms': ('total_ms', 'p95'),
    'gateway_cpu_ms_per_success': ('resources', 'gateway', 'cpu_ms_per_success'),
    'gateway_peak_rss_mib': ('resources', 'gateway', 'peak_rss_mib'),
}
CELL_FIELDS = ('profile', 'concurrency', 'offered', 'chunks', 'chunk_bytes', 'pace_ms', 'rate', 'direct')


def metric(trial: dict, path: tuple[str, ...], *, allow_zero: bool = False) -> float:
    """metric validates finite measurements; only content-gap metrics may legitimately be zero."""
    value = trial
    for part in path:
        value = value[part]
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value) or value < 0 or (value == 0 and not allow_zero):
        raise ValueError(f'invalid metric {path}: {value!r}')
    return float(value)


def expected_cells(summary: dict, repeats: int) -> set[tuple]:
    """expected_cells requires explicit completion and qualification, then reconstructs the entire planned matrix."""
    if summary.get('schema_version') != 2 or summary.get('complete') is not True:
        raise ValueError('a complete schema-version-2 experiment is required')
    config = summary.get('configuration', {})
    if config.get('repeats') != repeats or config.get('skip_qualification') is not False:
        raise ValueError('repetition count must match the plan and correctness qualification must be enabled')
    for label in ('baseline', 'candidate'):
        proof = summary.get('qualification', {}).get(label, {})
        expected = {'normal': 0, 'crlf': 0, 'fragmented': 0, 'missing-done': 8, 'wrong-content': 8, 'malformed': 8,
                    'cancellation': '8/8 producers released within 5 seconds'}
        if proof != expected:
            raise ValueError('missing or failed correctness qualification')
    try:
        levels = [int(value) for value in config['concurrency'].split(',')]
        profiles = config['profiles'].split(',')
        if not levels or not profiles or len(set(levels)) != len(levels) or len(set(profiles)) != len(profiles):
            raise ValueError('empty or duplicate planned dimensions')
        if any(c < 1 or c > 4096 for c in levels) or set(profiles) - {'saturated', 'paced'}:
            raise ValueError('invalid planned dimensions')
        return {(profile, concurrency,
                 max(config['requests'] if profile == 'saturated' else config['paced_requests'], concurrency * 2),
                 config['chunks'] if profile == 'saturated' else config['paced_chunks'], config['chunk_bytes'],
                 0 if profile == 'saturated' else config['pace_ms'], config['rate'], config['direct'])
                for profile in profiles for concurrency in levels}
    except (KeyError, TypeError, AttributeError) as error:
        raise ValueError('incomplete experiment configuration') from error


def compare(summary: dict, expected_repeats: int) -> list[dict]:
    """compare validates the complete planned matrix, A/B pairs and billing before summarizing trial ratios."""
    if type(expected_repeats) is not int or expected_repeats < 2:
        raise ValueError('at least two repetitions are required')
    planned = expected_cells(summary, expected_repeats)
    trials = summary.get('trials', [])
    if not trials:
        raise ValueError('no trials')
    gap_present = ['max_inter_content_gap_ms' in trial for trial in trials]
    if any(gap_present) and not all(gap_present):
        raise ValueError('content-gap telemetry must be present in every trial or explicitly absent from all legacy trials')
    metrics = dict(METRICS)
    if all(gap_present):
        metrics['max_inter_content_gap_p95_ms'] = ('max_inter_content_gap_ms', 'p95')
    cells = defaultdict(dict)
    binaries = defaultdict(set)
    for trial in trials:
        label, repeat = trial['label'], trial['repeat']
        if label not in ('baseline', 'candidate') or type(repeat) is not int or not 0 <= repeat < expected_repeats:
            raise ValueError('unexpected variant or repetition')
        if trial['failed'] or trial['dropped'] or trial['completed'] != trial['offered']:
            raise ValueError('failed/dropped traffic cannot support an all-success performance comparison')
        if trial['offered'] <= 0:
            raise ValueError('empty workload')
        if not trial['direct'] and (trial['billing']['requests'] != trial['completed'] or trial['billing']['used_quota'] <= 0):
            raise ValueError('durable billing does not match successful traffic')
        key = tuple(trial[field] for field in CELL_FIELDS)
        if (label, repeat) in cells[key]:
            raise ValueError('duplicate variant/repetition')
        cells[key][label, repeat] = trial
        binaries[label].add(trial['binary_sha256'])
    if set(cells) != planned:
        raise ValueError('missing planned cells or mismatched workload configuration')
    if any(len(values) != 1 for values in binaries.values()):
        raise ValueError('a variant changed binaries during the experiment')
    expected = {(label, repeat) for label in ('baseline', 'candidate') for repeat in range(expected_repeats)}
    result = []
    for key, pairs in sorted(cells.items()):
        if set(pairs) != expected:
            raise ValueError('missing A/B pairs or mismatched workload configuration')
        for repeat in range(expected_repeats):
            a, b = pairs['baseline', repeat], pairs['candidate', repeat]
            if a['billing'] != b['billing']:
                raise ValueError('baseline/candidate durable billing differs')
        cell = {**dict(zip(CELL_FIELDS, key)), 'pairs': expected_repeats, 'metrics': {}}
        for name, path in metrics.items():
            allow_zero = name == 'max_inter_content_gap_p95_ms'
            baseline = [metric(pairs['baseline', repeat], path, allow_zero=allow_zero) for repeat in range(expected_repeats)]
            candidate = [metric(pairs['candidate', repeat], path, allow_zero=allow_zero) for repeat in range(expected_repeats)]
            deltas = [(b / a - 1) * 100 for a, b in zip(baseline, candidate)] if all(baseline) else []
            cell['metrics'][name] = {'baseline_median': statistics.median(baseline),
                'candidate_median': statistics.median(candidate),
                'paired_change_abs_median': statistics.median(b - a for a, b in zip(baseline, candidate)),
                'paired_change_pct_median': statistics.median(deltas) if deltas else None,
                'paired_change_pct_min': min(deltas) if deltas else None,
                'paired_change_pct_max': max(deltas) if deltas else None}
        result.append(cell)
    return result


def markdown(cells: list[dict]) -> str:
    """markdown renders median run-level observations and paired changes without significance claims."""
    lines = ['# Paired streaming E2E comparison', '',
        'Values are medians across independent runs. Changes are medians of paired percentage changes, not ratios of pooled requests.',
        'Small repeat counts are descriptive evidence, not proof of statistical significance or production capacity.', '',
        '| Profile | Concurrency | Pairs | RPS baseline / candidate | Paired RPS change | CPU ms/request baseline / candidate | Paired CPU change | p95 completion ms baseline / candidate | RSS MiB baseline / candidate |',
        '| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |']
    for cell in cells:
        metrics = cell['metrics']
        def values(name: str) -> str:
            """values formats baseline and candidate medians for one metric."""
            return f"{metrics[name]['baseline_median']:.2f} / {metrics[name]['candidate_median']:.2f}"
        lines.append(f"| {cell['profile']} | {cell['concurrency']} | {cell['pairs']} | {values('successful_rps')} | "
            f"{metrics['successful_rps']['paired_change_pct_median']:+.2f}% | {values('gateway_cpu_ms_per_success')} | "
            f"{metrics['gateway_cpu_ms_per_success']['paired_change_pct_median']:+.2f}% | {values('completion_p95_ms')} | {values('gateway_peak_rss_mib')} |")
    lines.extend(['', '## First-content latency and repeatability', '',
        '| Profile | Concurrency | p95 TTFT ms baseline / candidate | Paired RPS change range | Paired CPU change range |',
        '| --- | ---: | ---: | ---: | ---: |'])
    for cell in cells:
        m = cell['metrics']
        lines.append(f"| {cell['profile']} | {cell['concurrency']} | {m['ttft_p95_ms']['baseline_median']:.2f} / "
            f"{m['ttft_p95_ms']['candidate_median']:.2f} | {m['successful_rps']['paired_change_pct_min']:+.2f}% to "
            f"{m['successful_rps']['paired_change_pct_max']:+.2f}% | {m['gateway_cpu_ms_per_success']['paired_change_pct_min']:+.2f}% to "
            f"{m['gateway_cpu_ms_per_success']['paired_change_pct_max']:+.2f}% |")
    if cells and 'max_inter_content_gap_p95_ms' in cells[0]['metrics']:
        lines.extend(['', '## Streaming continuity', '',
            'Each request records its largest gap between observed nonempty content deltas. The metric below is the p95 of those request maxima, not a pooled token-gap percentile. First-content latency remains separate.',
            'Zero-baseline percentage changes are undefined; absolute paired differences remain available.', '',
            '| Profile | Concurrency | p95 request-max gap ms baseline / candidate | Paired absolute change ms | Paired change |',
            '| --- | ---: | ---: | ---: | ---: |'])
        for cell in cells:
            gap = cell['metrics']['max_inter_content_gap_p95_ms']
            change = gap['paired_change_pct_median']
            percent = 'undefined (zero baseline)' if change is None else f'{change:+.2f}%'
            lines.append(f"| {cell['profile']} | {cell['concurrency']} | {gap['baseline_median']:.2f} / {gap['candidate_median']:.2f} | {gap['paired_change_abs_median']:+.2f} | {percent} |")
    else:
        lines.extend(['', 'Content-gap telemetry was not captured in this legacy study; no continuity conclusion can be inferred.'])
    return '\n'.join(lines) + '\n'


def main() -> None:
    """main validates a saved summary and writes reproducible Markdown and optional JSON comparisons."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('summary', type=Path)
    parser.add_argument('--expected-repeats', type=int, required=True)
    parser.add_argument('--json-output', type=Path)
    args = parser.parse_args()
    cells = compare(json.loads(args.summary.read_text()), args.expected_repeats)
    if args.json_output:
        args.json_output.write_text(json.dumps(cells, indent=2) + '\n')
    print(markdown(cells), end='')


if __name__ == '__main__':
    main()
