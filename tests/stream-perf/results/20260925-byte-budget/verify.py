#!/usr/bin/env python3
"""Check the committed final performance ledger, not the separately archived raw qualification."""
from __future__ import annotations
import csv
import hashlib
import io
import itertools
import json
import math
from pathlib import Path
import statistics

METRICS = ('rps', 'cpu_ms', 'rss_mib', 'ttft_p95_ms', 'completion_p95_ms', 'gap_p95_ms')


def require(condition: bool, message: str) -> None:
    """require rejects invalid evidence even when assertions are disabled."""
    if not condition:
        raise ValueError(message)


def compare(rows: list[dict]) -> dict:
    """compare requires all 40 unique runs and recomputes performance gates without inventing billing proof."""
    cells = {}
    for row in rows:
        key = row['profile'], int(row['concurrency']), row['variant'], int(row['repeat'])
        require(key not in cells, 'duplicate run')
        for metric in METRICS:
            number = float(row[metric])
            require(math.isfinite(number) and number >= 0 and (number > 0 or metric == 'gap_p95_ms'), 'invalid metric')
        cells[key] = row
    expected = set(itertools.product(('paced', 'saturated'), (8, 64), ('baseline', 'candidate'), range(5)))
    require(set(cells) == expected, 'incomplete or unknown matrix')
    reports, violations, material = [], [], []
    for profile, concurrency in itertools.product(('paced', 'saturated'), (8, 64)):
        report = {'profile': profile, 'concurrency': concurrency, 'metrics': {}}
        for metric in METRICS:
            baseline = [float(cells[profile, concurrency, 'baseline', i][metric]) for i in range(5)]
            candidate = [float(cells[profile, concurrency, 'candidate', i][metric]) for i in range(5)]
            absolute = statistics.median(y - x for x, y in zip(baseline, candidate))
            percent = statistics.median((y / x - 1) * 100 for x, y in zip(baseline, candidate)) if all(baseline) else None
            favorable = sum(y > x if metric == 'rps' else y < x for x, y in zip(baseline, candidate))
            report['metrics'][metric] = {'baseline_median': statistics.median(baseline), 'candidate_median': statistics.median(candidate),
                'paired_abs_median': absolute, 'paired_pct_median': percent, 'favorable_pairs': favorable}
            if profile == 'saturated' and metric in ('rps', 'cpu_ms'):
                if (percent >= 5 if metric == 'rps' else percent <= -5) and favorable >= 4:
                    material.append([profile, concurrency, metric])
            short_bad = profile == 'paced' and ((metric == 'rps' and percent < -5) or (metric == 'cpu_ms' and percent > 5))
            rss_bad = metric == 'rss_mib' and percent > 10
            latency_bad = metric.endswith('_p95_ms') and absolute > 5 and (percent is None or percent > 10)
            if short_bad or rss_bad or latency_bad:
                violations.append([profile, concurrency, metric])
        reports.append(report)
    return {'performance_gates_pass': bool(material) and not violations, 'raw_qualification_reverified': False,
            'trials': len(rows), 'material_benefits': material, 'violations': violations, 'cells': reports}


def verify(root: Path) -> dict:
    """verify checks the manifest-bound file before evaluating the rounded ledger."""
    metadata = json.loads((root / 'manifest.json').read_text())
    data = (root / 'final-runs.csv').read_bytes()
    require(hashlib.sha256(data).hexdigest() == metadata['final_run_ledger_sha256'], 'ledger checksum mismatch')
    result = compare(list(csv.DictReader(io.StringIO(data.decode()))))
    require(result['performance_gates_pass'] is metadata['final_confirmation']['accepted'], 'published decision mismatch')
    return result


if __name__ == '__main__':
    print(json.dumps(verify(Path(__file__).resolve().parent), indent=2, allow_nan=False))
