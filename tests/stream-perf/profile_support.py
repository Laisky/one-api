"""Linux process and cgroup evidence for isolated sustained streaming diagnostics."""
from __future__ import annotations
import json
import os
from pathlib import Path
import time

import run
from profile_window import stable_window


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
