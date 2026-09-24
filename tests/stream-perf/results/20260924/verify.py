#!/usr/bin/env python3
"""Validate the committed run-level evidence and reproduce paired comparisons without third-party packages."""
from collections import defaultdict
import csv
import hashlib
import json
import math
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
import report


def main() -> None:
    """main verifies evidence hashes, traffic accounting, fixture shape and pair completeness before printing results."""
    root = Path(__file__).resolve().parent
    manifest = json.loads((root / 'manifest.json').read_text())
    data = (root / 'runs.csv').read_bytes()
    if hashlib.sha256(data).hexdigest() != manifest['runs_csv_sha256']:
        raise ValueError('run-level evidence hash mismatch')
    summaries = defaultdict(lambda: {'complete': True, 'trials': []})
    seen = set()
    for row in csv.DictReader(data.decode().splitlines()):
        experiment, label = row['experiment'], row['label']
        config = manifest['experiments'][experiment]
        repeat, concurrency = int(row['repeat']), int(row['concurrency'])
        identity = (experiment, label, repeat, row['profile'], concurrency)
        if identity in seen:
            raise ValueError('duplicate run')
        seen.add(identity)
        if label not in config['variants'] or not 0 <= repeat < config['repeats'] or concurrency not in config['concurrency'] or row['profile'] not in config['profiles']:
            raise ValueError('unexpected run configuration')
        offered, completed, failed, dropped, billed, quota = [int(row[key]) for key in (
            'offered', 'completed', 'failed', 'dropped', 'billed_requests', 'used_quota')]
        expected_requests = max(config['requests'][row['profile']], concurrency * 2)
        if min(offered, completed, failed, dropped, billed, quota) < 0 or offered != expected_requests or offered != completed + failed + dropped:
            raise ValueError('invalid traffic accounting')
        if billed != (0 if config['direct'] else completed) or (not config['direct'] and completed and quota <= 0):
            raise ValueError('durable billing mismatch')
        values = {key: float(row[key]) for key in ('successful_rps', 'ttft_p95_ms', 'total_p95_ms', 'gateway_cpu_ms_per_success', 'gateway_peak_rss_mib')}
        if any(not math.isfinite(value) or value < 0 for value in values.values()):
            raise ValueError('invalid measurement')
        trial = {'label': label, 'repeat': repeat, 'profile': row['profile'], 'concurrency': concurrency,
            'offered': offered, 'completed': completed, 'failed': failed, 'dropped': dropped,
            'billing': {'requests': billed, 'used_quota': quota}, 'direct': config['direct'],
            'rate': config['rate'], 'chunks': config['chunks'][row['profile']], 'chunk_bytes': 128,
            'pace_ms': 0 if row['profile'] == 'saturated' else 2,
            'binary_sha256': manifest['binaries'][config['variants'][label]]['sha256'],
            'successful_rps': values['successful_rps'], 'ttft_ms': {'p95': values['ttft_p95_ms']},
            'total_ms': {'p95': values['total_p95_ms']}, 'resources': {'gateway': {
                'cpu_ms_per_success': values['gateway_cpu_ms_per_success'], 'peak_rss_mib': values['gateway_peak_rss_mib']}}}
        summaries[experiment]['trials'].append(trial)
    if set(summaries) != set(manifest['experiments']):
        raise ValueError('missing experiment')
    for experiment, config in manifest['experiments'].items():
        summary = summaries[experiment]
        expected = config['repeats'] * len(config['variants']) * len(config['concurrency']) * len(config['profiles'])
        if len(summary['trials']) != expected:
            raise ValueError('incomplete experiment')
        print(f'## {experiment}: {expected} validated runs')
        if len(config['variants']) == 2:
            cells = report.compare(summary, config['repeats'])
            print(report.markdown(cells))
            for cell in cells:
                print('Paired latency evidence:', cell['profile'], cell['concurrency'], json.dumps({
                    key: cell['metrics'][key] for key in ('ttft_p95_ms', 'completion_p95_ms')}))
        else:
            print('Traffic:', {key: sum(t[key] for t in summary['trials']) for key in ('offered', 'completed', 'failed', 'dropped')})
    print(f'Validated all {len(seen)} run-level records.')


if __name__ == '__main__':
    main()
