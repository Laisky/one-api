#!/usr/bin/env python3
"""Verify the committed follow-up run ledger, qualification and immutable provenance."""
from __future__ import annotations
import csv
import hashlib
import json
import math
from pathlib import Path
import statistics

ROOT = Path(__file__).resolve().parent
IDENTITY = {'study', 'label', 'profile'}
COUNTS = {'repeat', 'concurrency', 'rate', 'offered', 'completed', 'failed', 'dropped',
          'billing_requests', 'billing_used_quota'}
METRICS = ('rps', 'cpu_ms', 'rss_mib', 'ttft_p95_ms', 'total_p95_ms')


def read_rows(path: Path) -> list[dict]:
    """read_rows parses the run ledger without silently converting absent measurements to zero."""
    with path.open(newline='') as source:
        return [{key: value if key in IDENTITY else int(value) if key in COUNTS else float(value)
                 for key, value in row.items()} for row in csv.DictReader(source)]


def validate(manifest: dict, rows: list[dict]) -> None:
    """validate requires complete planned cells, qualified binaries, exact traffic and paired billing."""
    studies = {study['name']: study for study in manifest['studies']}
    if len(studies) != len(manifest['studies']):
        raise ValueError('duplicate study')
    expected = set()
    for name, study in studies.items():
        config = study['configuration']
        if study['complete'] is not True or config['skip_qualification']:
            raise ValueError('missing qualification or incomplete study')
        labels = set(study['binaries'])
        if set(study['qualification']) != labels:
            raise ValueError('missing qualification variant')
        for proof in study['qualification'].values():
            if any(proof.get(k) != 0 for k in ('normal', 'crlf', 'fragmented')) or any(
                    proof.get(k) != 8 for k in ('missing-done', 'wrong-content', 'malformed')):
                raise ValueError('qualification controls failed')
            if proof.get('cancellation') != '8/8 producers released within 5 seconds':
                raise ValueError('cancellation qualification failed')
        if study['driver_sha256'] != manifest['driver_sha256']:
            raise ValueError('driver provenance mismatch')
        for binary in study['binaries'].values():
            if manifest['binaries'][binary['id']]['sha256'] != binary['sha256']:
                raise ValueError('binary provenance mismatch')
        expected.update((name, label, profile, concurrency, repeat)
                        for label in labels for profile in config['profiles']
                        for concurrency in config['concurrency'] for repeat in range(config['repeats']))
    seen, pairs = set(), {}
    for row in rows:
        key = tuple(row[k] for k in ('study', 'label', 'profile', 'concurrency', 'repeat'))
        if key not in expected or key in seen:
            raise ValueError('unexpected or duplicate trial')
        seen.add(key)
        study = studies[row['study']]
        config = study['configuration']
        requests = config['requests'] if row['profile'] == 'saturated' else config['paced_requests']
        if row['rate'] != config['rate'] or row['offered'] != max(requests, row['concurrency'] * 2):
            raise ValueError('workload mismatch')
        if row['failed'] or row['offered'] != row['completed'] + row['failed'] + row['dropped']:
            raise ValueError('failed or unaccounted traffic')
        if any(row[k] < 0 for k in COUNTS) or not row['completed']:
            raise ValueError('invalid counters')
        if row['dropped'] and not config['allow_overload']:
            raise ValueError('unexpected admission drops')
        billed = 0 if config['direct'] else row['completed']
        if row['billing_requests'] != billed or (billed and row['billing_used_quota'] <= 0):
            raise ValueError('durable billing mismatch')
        if config['direct'] and row['billing_used_quota']:
            raise ValueError('direct workload must not charge gateway quota')
        for metric in METRICS + ('seconds', 'cpu_seconds', 'mock_cpu_seconds', 'driver_cpu_seconds',
                                  'mock_rss_mib', 'done_p95_ms', 'scheduled_p95_ms'):
            if not math.isfinite(row[metric]) or row[metric] <= 0:
                raise ValueError('invalid measurement')
        if not math.isclose(row['rps'] * row['seconds'], row['completed'], rel_tol=1e-7):
            raise ValueError('throughput denominator mismatch')
        if not math.isclose(row['cpu_ms'] * row['completed'], row['cpu_seconds'] * 1000, rel_tol=1e-7):
            raise ValueError('CPU denominator mismatch')
        if study['kind'] == 'paired':
            pair_key = (row['study'], row['profile'], row['concurrency'], row['repeat'])
            pairs.setdefault(pair_key, {})[row['label']] = row
    if seen != expected:
        raise ValueError('missing planned trials')
    for pair in pairs.values():
        if set(pair) != {'baseline', 'candidate'}:
            raise ValueError('missing paired variant')
        if any(pair['baseline'][key] != pair['candidate'][key]
               for key in ('completed', 'billing_requests', 'billing_used_quota')):
            raise ValueError('paired billing mismatch')
    totals = {'trials': len(rows), **{k: sum(row[k] for row in rows)
                                     for k in ('offered', 'completed', 'failed', 'dropped')}}
    if totals != manifest['totals']:
        raise ValueError('published totals mismatch')


def comparisons(rows: list[dict], study: str) -> list[dict]:
    """comparisons reports per-run medians and matched-pair changes without pooling requests or studies."""
    selected = [row for row in rows if row['study'] == study]
    result = []
    for profile, concurrency in sorted({(row['profile'], row['concurrency']) for row in selected}):
        cell = [row for row in selected if row['profile'] == profile and row['concurrency'] == concurrency]
        baseline = sorted((row for row in cell if row['label'] == 'baseline'), key=lambda r: r['repeat'])
        candidate = sorted((row for row in cell if row['label'] == 'candidate'), key=lambda r: r['repeat'])
        if not baseline or [r['repeat'] for r in baseline] != [r['repeat'] for r in candidate]:
            raise ValueError('comparison requires matched pairs')
        metrics = {}
        for metric in METRICS:
            changes = [100 * (b[metric] / a[metric] - 1) for a, b in zip(baseline, candidate)]
            metrics[metric] = {'baseline_median': statistics.median(r[metric] for r in baseline),
                               'candidate_median': statistics.median(r[metric] for r in candidate),
                               'paired_change_pct_median': statistics.median(changes),
                               'paired_change_pct_min': min(changes), 'paired_change_pct_max': max(changes)}
        result.append(dict(profile=profile, concurrency=concurrency, pairs=len(baseline), metrics=metrics))
    return result


def load(root: Path = ROOT) -> tuple[dict, list[dict]]:
    """load checks stored-byte hashes before trusting the manifest's associated ledger and explanatory files."""
    manifest = json.loads((root / 'manifest.json').read_text())
    for name, digest in manifest['files'].items():
        path = root / name
        if Path(name).name != name or not path.is_file():
            raise ValueError('missing or unsafe evidence path')
        if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
            raise ValueError(f'checksum mismatch: {name}')
    rows = read_rows(root / 'runs.csv')
    validate(manifest, rows)
    return manifest, rows


def main() -> None:
    """main verifies the complete ledger and prints all paired results and traffic accounting."""
    manifest, rows = load()
    output = {study['name']: comparisons(rows, study['name']) for study in manifest['studies']
              if study['kind'] == 'paired'}
    output['totals'] = manifest['totals']
    print(json.dumps(output, indent=2))


if __name__ == '__main__':
    main()
