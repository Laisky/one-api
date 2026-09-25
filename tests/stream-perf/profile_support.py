"""Linux process and cgroup evidence for isolated sustained streaming diagnostics."""
from __future__ import annotations
import json
import math
import os
from pathlib import Path
import statistics
import time

import run


def cpu_allowance(cgroup: Path = Path('/sys/fs/cgroup'), affinity: set[int] | None = None) -> dict:
    """cpu_allowance records visible cgroup-v2 limits and affinity without treating host CPUs as usable capacity."""
    affinity = os.sched_getaffinity(0) if affinity is None else affinity
    if not affinity:
        raise ValueError('empty CPU affinity')
    limits = []
    # Walk the process cgroup and every visible ancestor; namespace-hidden limits remain an explicit boundary.
    relative = next(line.split(':', 2)[2] for line in Path('/proc/self/cgroup').read_text().splitlines()
                    if line.startswith('0::'))
    current = cgroup / relative.lstrip('/')
    if not current.is_relative_to(cgroup):
        raise ValueError('invalid cgroup path')
    while True:
        path = current / 'cpu.max'
        if path.exists():
            quota, period = path.read_text().split()
            if int(period) <= 0 or (quota != 'max' and int(quota) <= 0):
                raise ValueError('invalid CPU quota')
            limits.append({'path': str(path), 'raw': f'{quota} {period}',
                           'cores': None if quota == 'max' else int(quota) / int(period)})
        if current == cgroup:
            break
        current = current.parent
    if not limits:
        raise RuntimeError('cgroup-v2 CPU accounting is required')
    return {'effective_cores': min([float(len(affinity)), *[x['cores'] for x in limits if x['cores'] is not None]]),
            'affinity': sorted(affinity), 'visible_limits': limits,
            'boundary': 'Only namespace-visible cgroup ancestors can be inspected.',
            'stat_path': str(cgroup / relative.lstrip('/') / 'cpu.stat'),
            'memory_limit': (cgroup / relative.lstrip('/') / 'memory.max').read_text().strip()}


def cgroup_stats(path: Path) -> dict[str, int]:
    """cgroup_stats reads cumulative shared-cgroup CPU counters without attributing them to the gateway."""
    return {key: int(value) for key, value in (line.split() for line in path.read_text().splitlines())}


def snapshot(pids: dict[str, int], mock_url: str, database: Path, stat_path: Path, started: float) -> dict:
    """snapshot records process CPU/RSS, upstream progress and durable usage, keeping their different meanings explicit."""
    at = time.monotonic()
    processes = {name: dict(zip(('cpu_seconds', 'rss_bytes'), run.proc_sample(pid))) for name, pid in pids.items()}
    return {'elapsed': at - started, 'processes': processes, 'upstream': run.api(mock_url + '/health'),
            'durable_requests': run.usage_snapshot(database)[1], 'cgroup': cgroup_stats(stat_path)}


def stable_window(rows: list[dict], cores: float, gateway_procs: int, start: float, seconds: float,
                  minimum: float = .5, fraction: float = .8) -> dict:
    """stable_window evaluates gateway-only CPU admission and publishes progress/drift without replacing failed intervals."""
    if not math.isfinite(cores) or cores <= 0 or gateway_procs < 1:
        raise ValueError('invalid CPU allowance')
    intervals = []
    for previous, current in zip(rows, rows[1:]):
        if previous['elapsed'] < start or current['elapsed'] > start + seconds + .25:
            continue
        duration = current['elapsed'] - previous['elapsed']
        cpu = current['processes']['gateway']['cpu_seconds'] - previous['processes']['gateway']['cpu_seconds']
        if duration <= 0 or cpu < 0:
            raise ValueError('non-monotonic samples')
        used = cpu / duration
        intervals.append({'start': previous['elapsed'], 'end': current['elapsed'], 'seconds': duration,
                          'gateway_cores': used, 'machine_utilization': used / cores,
                          'gateway_slot_utilization': used / min(cores, gateway_procs),
                          'upstream_completed': current['upstream']['completed'] - previous['upstream']['completed'],
                          'durable_requests': current['durable_requests'] - previous['durable_requests'],
                          'upstream_active': current['upstream']['active']})
    coverage = sum(x['seconds'] for x in intervals)
    ratio = sum(x['machine_utilization'] >= minimum for x in intervals) / len(intervals) if intervals else 0
    half = len(intervals) // 2
    means = [statistics.mean(x['gateway_cores'] for x in part) for part in (intervals[:half], intervals[half:]) if part]
    drift = abs(means[-1] / means[0] - 1) if len(means) == 2 and means[0] else None
    qualified = bool(intervals) and coverage >= seconds - 2 and max(x['seconds'] for x in intervals) <= 2.5 and ratio >= fraction
    return {'qualified': qualified, 'intervals': intervals, 'coverage_seconds': coverage,
            'fraction_at_or_above_target': ratio, 'target_machine_utilization': minimum,
            'required_fraction': fraction, 'gateway_cores_mean': statistics.mean(x['gateway_cores'] for x in intervals) if intervals else None,
            'half_window_gateway_cores': means, 'relative_cpu_drift': drift,
            'upstream_active_peak': max((x['upstream_active'] for x in intervals), default=None),
            'progress_note': 'Upstream completions and durable statistics are proxies, not client completion timestamps.'}


def assert_loopback_listeners(pid: int, expected_ports: set[int]) -> None:
    """assert_loopback_listeners rejects missing or externally bound fixture sockets using the gateway's actual descriptors."""
    inodes = set()
    for descriptor in Path(f'/proc/{pid}/fd').iterdir():
        try:
            value = os.readlink(descriptor)
        except FileNotFoundError:
            continue
        if value.startswith('socket:['):
            inodes.add(value[8:-1])
    found = set()
    for family in ('tcp', 'tcp6'):
        for line in Path(f'/proc/{pid}/net/{family}').read_text().splitlines()[1:]:
            fields = line.split()
            if fields[3] != '0A' or fields[9] not in inodes:
                continue
            address, port = fields[1].split(':')
            number = int(port, 16)
            if number not in expected_ports:
                raise RuntimeError('unexpected gateway listener')
            if (family, address) != ('tcp', '0100007F'):
                raise RuntimeError('gateway listener is not IPv4 loopback')
            found.add(number)
    if found != expected_ports:
        raise RuntimeError('expected loopback gateway listeners are missing')


def write_json(path: Path, data: dict) -> None:
    """write_json atomically checkpoints credential-free diagnostic evidence."""
    temporary = path.with_suffix(path.suffix + '.tmp')
    temporary.write_text(json.dumps(data, indent=2, allow_nan=False) + '\n')
    temporary.replace(path)
