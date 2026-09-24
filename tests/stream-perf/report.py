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


def metric(trial: dict, path: tuple[str, ...]) -> float:
    """metric reads one strictly positive finite measurement or rejects invalid evidence."""
    value = trial
    for part in path:
        value = value[part]
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value) or value <= 0:
        raise ValueError(f'invalid metric {path}: {value!r}')
    return float(value)


def compare(summary: dict, expected_repeats: int) -> list[dict]:
    """compare validates complete A/B pairs and billing, then summarizes independent trial ratios."""
    if expected_repeats < 2 or summary.get('complete') is False:
        raise ValueError('a complete experiment with at least two repetitions is required')
    trials = summary.get('trials', [])
    if not trials:
        raise ValueError('no trials')
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
        for name, path in METRICS.items():
            baseline = [metric(pairs['baseline', repeat], path) for repeat in range(expected_repeats)]
            candidate = [metric(pairs['candidate', repeat], path) for repeat in range(expected_repeats)]
            deltas = [(b / a - 1) * 100 for a, b in zip(baseline, candidate)]
            cell['metrics'][name] = {'baseline_median': statistics.median(baseline),
                'candidate_median': statistics.median(candidate), 'paired_change_pct_median': statistics.median(deltas),
                'paired_change_pct_min': min(deltas), 'paired_change_pct_max': max(deltas)}
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
