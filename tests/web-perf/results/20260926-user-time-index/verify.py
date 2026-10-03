#!/usr/bin/env python3
"""Verify the exact Web API confirmation and optionally every original request observation."""
from __future__ import annotations
import argparse
import csv
import gzip
import hashlib
import io
import json
import math
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
import report
from http_fixture import require


def digest(data: bytes) -> str:
    """digest identifies exact persisted bytes rather than a reformatted summary."""
    return hashlib.sha256(data).hexdigest()


def validate_provenance(summary: dict, manifest: dict) -> None:
    """validate_provenance binds the measured binaries, fixture, endpoint contract and unchanged schema candidate."""
    require(summary['binaries'] == manifest['binaries'], 'binary identity mismatch')
    require(summary['dataset']['logical_sha256'] == manifest['dataset_sha256'], 'fixture identity mismatch')
    require(summary['dataset']['rows'] == 200000, 'fixture size mismatch')
    for label in ('baseline', 'candidate'):
        schema = summary['schema'][label]
        require(schema['index_columns'] == (['user_id', 'created_at', 'id'] if label == 'candidate' else []), 'index column mismatch')
        freshness = summary['freshness'][label]
        require(all(freshness.get(k) is True for k in ('committed_insert', 'legacy_rows_and_count', 'all_dashboard_aggregates')), 'incomplete freshness evidence')
        qualification = summary['qualification'][label]
        require(qualification['filtered_exact_rows'] > 0, 'selective fixture missing')
        require(set(qualification['first_hit_ms']) == report.TARGETS | report.CONTROLS, 'first-hit evidence missing')
    expected_order = [(e, c, label, repeat) for repeat in range(5) for c in (1, 8)
                      for label in (('baseline', 'candidate') if repeat % 2 == 0 else ('candidate', 'baseline'))
                      for e in ('self-dashboard', 'self-logs', 'self-deep', 'self-cursor', 'site-dashboard', 'admin-logs', 'admin-stat', 'user-self')]
    observed = [(t['endpoint'], t['concurrency'], t['label'], t['repeat']) for t in summary['trials']]
    require(observed == expected_order, 'trial execution order mismatch')
    for trial in summary['trials']:
        require(trial['path'] == manifest['endpoints'][trial['endpoint']]['path'], 'endpoint path mismatch')
        require(trial['principal'] == manifest['endpoints'][trial['endpoint']]['principal'], 'principal mismatch')
        for group in ('gateway', 'client'):
            for key, value in trial['resources'][group].items():
                require(type(value) in (int, float) and math.isfinite(value) and value >= 0, 'invalid resource observation')
        cpu = trial['resources']['gateway']
        require(math.isclose(cpu['cpu_ms_per_request'], cpu['cpu_seconds'] * 1000 / trial['completed'], rel_tol=1e-12, abs_tol=1e-12), 'CPU/request arithmetic mismatch')


def verify_requests(summary: dict, directory: Path) -> int:
    """verify_requests recomputes all samples, warmups and summaries without claiming persisted timing proves payload contents."""
    result = report.audit(directory)
    require(result['raw_samples_verified'] is True, 'raw audit did not run')
    count = 0
    for trial in summary['trials']:
        name = f"{trial['label']}-c{trial['concurrency']}-r{trial['repeat']}-{trial['endpoint']}.json"
        raw = json.loads((directory / name).read_text())
        per_worker = trial['completed'] // trial['concurrency']
        for sample in raw['samples']:
            require(type(sample['index']) is int and type(sample['worker']) is int, 'invalid request identifier')
            require(sample['worker'] == sample['index'] // per_worker, 'worker/request identity mismatch')
            require(type(sample['response_bytes']) is int and 0 < sample['response_bytes'] <= 16 << 20, 'invalid body size')
        for warmup in raw['warmups']:
            require(type(warmup['worker']) is int and type(warmup['index']) is int, 'invalid warmup identifier')
            require(type(warmup['response_bytes']) is int and 0 < warmup['response_bytes'] <= 16 << 20, 'invalid warmup body size')
            report.finite(warmup['latency_ms'])
        count += len(raw['samples'])
    require(count == 51200, 'raw request total mismatch')
    return count


def verify(root: Path, raw_directory: Path | None = None) -> dict:
    """verify checks committed evidence integrity and independently recalculates the frozen acceptance decision."""
    manifest = json.loads((root / 'manifest.json').read_text())
    for name, expected in manifest['files'].items():
        require(Path(name).name == name, 'unexpected manifest path')
        require(digest((root / name).read_bytes()) == expected, 'file integrity mismatch: ' + name)
    data = gzip.decompress((root / 'summary.json.gz').read_bytes())
    require(digest(data) == manifest['summary_sha256'], 'summary byte identity mismatch')
    summary = json.loads(data)
    result = report.compare(summary)
    validate_provenance(summary, manifest)
    require(result['accepted'] is manifest['accepted'], 'published decision mismatch')
    require(result['completed'] == 51200 and result['trials'] == 160, 'published matrix mismatch')
    if raw_directory is not None:
        require(digest((raw_directory / 'summary.json').read_bytes()) == manifest['summary_sha256'], 'wrong raw study')
        result['verified_request_samples'] = verify_requests(summary, raw_directory)
        result['raw_samples_verified'] = True
    else:
        result['verified_request_samples'] = 0
    result['historical_32_request_study_raw_available'] = False
    result['scope'] = 'local SQLite HTTP responses; not browser paint, cold OS-cache, native PG/MySQL speed or production capacity'
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--raw-directory', type=Path)
    arguments = parser.parse_args()
    print(json.dumps(verify(Path(__file__).resolve().parent, arguments.raw_directory), indent=2, allow_nan=False))
