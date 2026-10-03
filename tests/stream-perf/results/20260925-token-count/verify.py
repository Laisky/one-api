#!/usr/bin/env python3
"""Audit both exact-count experiments without pooling or dropping unfavorable trials."""
import argparse
import csv
import gzip
import hashlib
import json
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
import report

FIELDS = {
    'label': ('label',), 'profile': ('profile',), 'concurrency': ('concurrency',), 'repeat': ('repeat',),
    'offered': ('offered',), 'completed': ('completed',), 'failed': ('failed',), 'dropped': ('dropped',),
    'rps': ('successful_rps',), 'cpu_ms': ('resources', 'gateway', 'cpu_ms_per_success'),
    'rss_mib': ('resources', 'gateway', 'peak_rss_mib'), 'ttft_p95_ms': ('ttft_ms', 'p95'),
    'completion_p95_ms': ('total_ms', 'p95'), 'gap_p95_ms': ('max_inter_content_gap_ms', 'p95'),
    'billing_requests': ('billing', 'requests'), 'billing_used_quota': ('billing', 'used_quota'),
}


def ledger(summary):
    """ledger derives every CSV field from the raw summary without rounding or omitting a trial."""
    rows = []
    for trial in summary['trials']:
        row = {}
        for name, path in FIELDS.items():
            value = trial
            for part in path:
                value = value[part]
            row[name] = str(value)
        rows.append(row)
    return rows


def decision(summary):
    """decision applies the predeclared four-cell operational gates, not a statistical significance test."""
    cells = report.compare(summary, 5)
    expected = {('paced', 8, 512, 32, 128, 2, 0, False), ('paced', 64, 512, 32, 128, 2, 0, False),
                ('saturated', 8, 256, 1024, 128, 0, 0, False), ('saturated', 64, 256, 1024, 128, 0, 0, False)}
    if {tuple(c[k] for k in report.CELL_FIELDS) for c in cells} != expected:
        raise ValueError('not the registered four-cell workload')
    reasons, benefits = [], []
    for cell in cells:
        name = f"{cell['profile']}/c{cell['concurrency']}"
        metrics = cell['metrics']
        if 'max_inter_content_gap_p95_ms' not in metrics:
            raise ValueError('continuity telemetry is required')
        if cell['profile'] == 'saturated':
            pairs = {(t['label'], t['repeat']): t for t in summary['trials']
                     if t['profile'] == cell['profile'] and t['concurrency'] == cell['concurrency']}
            for metric, direction in (('successful_rps', 1), ('gateway_cpu_ms_per_success', -1)):
                path = report.METRICS[metric]
                favorable = sum(direction * (report.metric(pairs['candidate', i], path) -
                                report.metric(pairs['baseline', i], path)) > 0 for i in range(5))
                if direction * metrics[metric]['paired_change_pct_median'] >= 5 and favorable >= 4:
                    benefits.append(f'{name}: {metric}')
        else:
            if metrics['successful_rps']['paired_change_pct_median'] < -5:
                reasons.append(f'{name}: short-stream throughput regressed')
            if metrics['gateway_cpu_ms_per_success']['paired_change_pct_median'] > 5:
                reasons.append(f'{name}: short-stream CPU regressed')
        for metric in ('ttft_p95_ms', 'completion_p95_ms', 'max_inter_content_gap_p95_ms'):
            value = metrics[metric]
            if value['paired_change_pct_median'] is None:
                raise ValueError('zero-baseline continuity requires a separately registered decision rule')
            if value['paired_change_pct_median'] > 10 and value['paired_change_abs_median'] > 5:
                reasons.append(f'{name}: {metric} exceeds both latency limits')
        if metrics['gateway_peak_rss_mib']['paired_change_pct_median'] > 10:
            reasons.append(f'{name}: RSS regressed')
    if not benefits:
        reasons.append('no long-stream metric meets both material benefit and 4/5 repeatability gates')
    return {'accepted': not reasons, 'reasons': reasons, 'benefits': benefits, 'cells': cells}


def verify(root, evidence):
    """verify binds each complete raw study to its identities, ledger and predeclared decision."""
    manifest = json.loads((root / 'manifest.json').read_text())
    for name, expected in manifest['files'].items():
        if Path(name).name != name or hashlib.sha256((root / name).read_bytes()).hexdigest() != expected:
            raise ValueError('published file integrity failure: ' + name)
    with gzip.open(root / 'runs.csv.gz', 'rt', newline='') as source:
        published = list(csv.DictReader(source))
    names = [study['name'] for study in manifest['studies']]
    if len(set(names)) != len(names) or set(row['study'] for row in published) != set(names):
        raise ValueError('study inventory mismatch')
    results = {}
    for study in manifest['studies']:
        path = (evidence / study['raw_summary']).resolve()
        if not path.is_relative_to(evidence.resolve()):
            raise ValueError('raw summary escapes evidence directory')
        raw = path.read_bytes()
        if hashlib.sha256(raw).hexdigest() != study['summary_sha256']:
            raise ValueError('raw summary integrity failure')
        summary = json.loads(raw)
        result = decision(summary)
        for trial in summary['trials']:
            if trial['binary_sha256'] != study['binaries'][trial['label']]['sha256']:
                raise ValueError('binary identity mismatch')
        if summary['environment']['driver_sha256'] != study['driver_sha256']:
            raise ValueError('driver identity mismatch')
        if summary['configuration'] != study['configuration'] or summary['qualification'] != study['qualification']:
            raise ValueError('workload or qualification mismatch')
        expected_rows = [{'study': study['name'], **row} for row in ledger(summary)]
        if [row for row in published if row['study'] == study['name']] != expected_rows:
            raise ValueError('ledger differs from raw trials')
        totals = {'trials': len(summary['trials']), **{k: sum(t[k] for t in summary['trials'])
                  for k in ('offered', 'completed', 'failed', 'dropped')}}
        if totals != study['totals'] or any(result[k] != study[k] for k in ('accepted', 'reasons', 'benefits')):
            raise ValueError('published totals or decision mismatch')
        results[study['name']] = {**result, 'totals': totals}
    return results


def main():
    """main requires both raw summaries from the evidence archive and recomputes the independent decisions."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(verify(Path(__file__).resolve().parent, args.evidence), indent=2))


if __name__ == '__main__':
    main()
