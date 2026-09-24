#!/usr/bin/env python3
"""Run reproducible, loopback-only black-box streaming experiments against real gateway binaries."""
from __future__ import annotations
import argparse
import contextlib
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import platform
import resource
import secrets
import socket
import sqlite3
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request


def free_port() -> int:
    """free_port returns an available loopback port; bind failures remain fatal rather than killing existing listeners."""
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def api(url: str, token: str = '', payload: dict | None = None) -> dict:
    """api sends a local management request without proxies and returns its decoded JSON response."""
    request = urllib.request.Request(url, data=None if payload is None else json.dumps(payload).encode(),
                                     headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
    with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(request, timeout=3) as response:
        return json.load(response)


def wait_ready(url: str, process: subprocess.Popen, seconds: float = 60) -> None:
    """wait_ready requires a responding local service within a deadline and fails promptly when it exits."""
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f'service exited with status {process.returncode}; inspect private fixture log')
        try:
            api(url)
            return
        except (urllib.error.URLError, TimeoutError, ConnectionError):
            time.sleep(.05)
    raise TimeoutError('local service readiness deadline exceeded')


def stop(process: subprocess.Popen) -> None:
    """stop terminates and reaps only a process created by this harness, with a bounded kill fallback."""
    if process.poll() is not None:
        return
    process.terminate()
    try:
        process.wait(timeout=15)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=5)


def proc_sample(pid: int) -> tuple[float, int]:
    """proc_sample returns Linux process CPU seconds and current resident bytes, excluding child processes."""
    fields = Path(f'/proc/{pid}/stat').read_text().rsplit(')', 1)[1].split()
    cpu = (int(fields[11]) + int(fields[12])) / os.sysconf('SC_CLK_TCK')
    rss = int(Path(f'/proc/{pid}/statm').read_text().split()[1]) * os.sysconf('SC_PAGE_SIZE')
    return cpu, rss


class Resources:
    """Resources samples gateway and mock RSS independently while taking cumulative CPU deltas."""
    def __init__(self, pids: dict[str, int]):
        """__init__ captures pre-trial CPU baselines and starts a bounded resource sampler."""
        self.pids, self.baseline = pids, {name: proc_sample(pid) for name, pid in pids.items()}
        self.peak = {name: value[1] for name, value in self.baseline.items()}
        self.done = threading.Event()
        self.error: Exception | None = None
        self.thread = threading.Thread(target=self.sample, daemon=True)
        self.thread.start()

    def sample(self) -> None:
        """sample records per-process resident high-water observations every 20 milliseconds until stopped."""
        try:
            while not self.done.wait(.02):
                for name, pid in self.pids.items():
                    self.peak[name] = max(self.peak[name], proc_sample(pid)[1])
        except (OSError, ValueError) as error:
            self.error = error

    def finish(self, completed: int, seconds: float) -> dict:
        """finish joins the sampler and returns CPU time and sampled RSS without converting missing data into zeros."""
        self.done.set()
        self.thread.join(timeout=2)
        if self.error:
            raise self.error
        out = {}
        for name, pid in self.pids.items():
            cpu, rss = proc_sample(pid)
            delta = cpu - self.baseline[name][0]
            out[name] = {'cpu_seconds': delta, 'cpu_ms_per_success': delta * 1000 / completed if completed else None,
                         'average_cores': delta / seconds, 'peak_rss_mib': max(rss, self.peak[name]) / 2**20}
        return out


def sha256(path: Path) -> str:
    """sha256 returns a streaming SHA-256 digest for a binary without loading it all into memory."""
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            digest.update(block)
    return digest.hexdigest()


def driver_command(args: argparse.Namespace, url: str, output: Path, trial: str,
                   concurrency: int, requests: int, chunks: int, pace: int, fault: str = '', cancel: int = 0) -> list[str]:
    """driver_command constructs bounded workload arguments, keeping all credentials out of command lines."""
    return [str(args.driver), '-url', url, '-output', str(output), '-id', trial,
            '-concurrency', str(concurrency), '-requests', str(requests), '-chunks', str(chunks),
            '-chunk-bytes', str(args.chunk_bytes), '-pace-ms', str(pace), '-timeout', '60s',
            '-fault', fault, '-cancel-after', str(cancel), '-rate', str(args.rate if not fault and not cancel else 0)]


@contextlib.contextmanager
def fixture(args: argparse.Namespace, binary: Path):
    """fixture provisions only temporary local state via real admin APIs and guarantees process cleanup."""
    with tempfile.TemporaryDirectory(prefix='oneapi-stream-') as directory:
        root = Path(directory)
        admin, token, upstream = (secrets.token_hex(24) for _ in range(3))
        base_env = {'PATH': os.environ.get('PATH', '/usr/bin:/bin'), 'HOME': directory, 'TZ': 'UTC', 'GOMAXPROCS': '2'}
        mock_port, gateway_port = free_port(), free_port()
        while mock_port == gateway_port:
            gateway_port = free_port()
        mock_url, gateway_url = f'http://127.0.0.1:{mock_port}', f'http://127.0.0.1:{gateway_port}'
        children = []
        with (root / 'mock.log').open('w') as mock_log, (root / 'gateway.log').open('w') as gateway_log:
            try:
                mock = subprocess.Popen([str(args.driver), '-mode', 'mock', '-listen', f'127.0.0.1:{mock_port}'],
                                        cwd=root, env={**base_env, 'STREAM_PERF_UPSTREAM_TOKEN': upstream},
                                        stdout=mock_log, stderr=subprocess.STDOUT)
                children.append(mock)
                wait_ready(mock_url + '/health', mock)
                environment = {**base_env, 'PORT': str(gateway_port), 'GIN_MODE': 'release',
                               'SQLITE_PATH': str(root / 'one-api.db'), 'TIKTOKEN_CACHE_DIR': str(args.token_cache),
                               'INITIAL_ROOT_ACCESS_TOKEN': admin, 'INITIAL_ROOT_TOKEN': token,
                               'GLOBAL_RELAY_RATE_LIMIT': '10000000', 'GLOBAL_API_RATE_LIMIT': '10000000'}
                gateway = subprocess.Popen([str(binary)], cwd=root, env=environment,
                                           stdout=gateway_log, stderr=subprocess.STDOUT)
                children.append(gateway)
                wait_ready(gateway_url + '/api/status', gateway)
                response = api(gateway_url + '/api/channel/', admin, {'name': 'stream-perf', 'type': 50,
                    'status': 1, 'key': upstream, 'base_url': mock_url, 'models': 'gpt-4o-mini', 'group': 'default'})
                if response.get('success') is not True:
                    raise RuntimeError('mock channel creation failed')
                yield {'url': gateway_url + '/v1/chat/completions', 'direct': mock_url + '/v1/chat/completions',
                       'mock_url': mock_url, 'env': {**base_env, 'STREAM_PERF_TOKEN': 'sk-' + token},
                       'upstream_env': {**base_env, 'STREAM_PERF_TOKEN': upstream},
                       'pids': {'gateway': gateway.pid, 'mock': mock.pid}, 'root': root}
            except Exception:
                # Never export the database or credentials. Optional diagnostics
                # redact every generated credential before retaining a failure log.
                if args.debug_logs:
                    destination = args.output / 'fixture-failure-redacted.log'
                    text = (root / 'gateway.log').read_text(errors='replace')
                    for value in (admin, token, upstream, '123456'):
                        text = text.replace(value, '[REDACTED]')
                    destination.write_text(text)
                raise
            finally:
                for process in reversed(children):
                    stop(process)


def run_driver(command: list[str], environment: dict, success: bool | None = True) -> dict:
    """run_driver executes the independent client, enforcing its status and returning its persisted observations."""
    before_cpu = resource.getrusage(resource.RUSAGE_CHILDREN)
    process = subprocess.run(command, env=environment, capture_output=True, text=True, timeout=600)
    after_cpu = resource.getrusage(resource.RUSAGE_CHILDREN)
    path = Path(command[command.index('-output') + 1])
    if not path.is_file():
        raise RuntimeError('driver did not produce a report: ' + process.stderr[-500:])
    report = json.loads(path.read_text())
    report['load_generator_cpu_seconds'] = after_cpu.ru_utime + after_cpu.ru_stime - before_cpu.ru_utime - before_cpu.ru_stime
    if success is not None and (process.returncode == 0) != success:
        failures = [sample.get('error') for sample in report['samples'] if sample.get('error')]
        raise RuntimeError(f'driver exit {process.returncode}; expected success={success}; failures={failures[:3]}')
    return report


def qualify(args: argparse.Namespace, binary: Path, label: str) -> dict:
    """qualify checks real-gateway delivery, injected truncation detection, authentication and cancellation before timing."""
    evidence = {}
    directory = args.output / ('correctness-' + label)
    directory.mkdir()
    with fixture(args, binary) as f:
        for fault in ('', 'crlf', 'fragmented', 'missing-done', 'wrong-content', 'malformed'):
            name = fault or 'normal'
            cmd = driver_command(args, f['url'], directory / f'{name}.json', name, 4, 8, 4, 1, fault)
            cmd[-1] = '0'  # Correctness qualification must not inherit an overload schedule.
            report = run_driver(cmd, f['env'], fault in ('', 'crlf', 'fragmented'))
            expected = {'missing-done': 'incomplete stream', 'wrong-content': 'content mismatch', 'malformed': 'invalid SSE JSON'}
            if fault in expected and any(expected[fault] not in sample.get('error', '') for sample in report['samples']):
                raise AssertionError('negative control failed for an unrelated reason')
            evidence[name] = report['failed']
        try:
            api(f['url'], 'invalid', {'model': 'gpt-4o-mini', 'stream': True, 'messages': []})
            raise AssertionError('unauthenticated request unexpectedly succeeded')
        except urllib.error.HTTPError as error:
            if error.code not in (401, 403):
                raise
        before = api(f['mock_url'] + '/health')['cancelled']
        cmd = driver_command(args, f['url'], directory / 'cancel.json', 'cancel', 4, 8, 1000, 5, cancel=1)
        run_driver(cmd, f['env'])
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            health = api(f['mock_url'] + '/health')
            if health['active'] == 0 and health['cancelled'] - before == 8:
                break
            time.sleep(.05)
        else:
            raise AssertionError('client cancellation did not release all upstream producers')
        evidence['cancellation'] = '8/8 producers released within 5 seconds'
    return evidence


def usage_snapshot(database: Path) -> tuple[int, int]:
    """usage_snapshot reads persisted billing aggregates without mutating gateway state."""
    with sqlite3.connect(f'file:{database}?mode=ro', uri=True, timeout=2) as connection:
        row = connection.execute("SELECT used_quota, request_count FROM users WHERE username = ?", ('root',)).fetchone()
    if row is None:
        raise AssertionError('fixture account disappeared')
    return row


def wait_usage(database: Path, count: int) -> tuple[int, int]:
    """wait_usage requires all completed requests to reach durable billing before accepting an experiment."""
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        usage = usage_snapshot(database)
        if usage[1] == count:
            return usage
        if usage[1] > count:
            raise AssertionError('more billed requests than offered; possible retry or duplicate billing')
        time.sleep(.01)
    raise AssertionError('completed requests did not settle in durable billing')


def measure(args: argparse.Namespace, binary: Path, label: str, repeat: int, concurrency: int, profile: str) -> dict:
    """measure warms a fresh gateway then times one workload, saving exact responses, errors and process-specific resources."""
    chunks, pace = (args.chunks, 0) if profile == 'saturated' else (args.paced_chunks, args.pace_ms)
    trial = f'{label}-{profile}-c{concurrency}-r{repeat}'
    with fixture(args, binary) as f:
        warm = driver_command(args, f['url'], f['root'] / 'warmup.json', 'warmup', min(concurrency, 8), 16, 8, 0)
        # Warmups are closed-loop regardless of the requested offered-load profile.
        warm[-1] = '0'
        run_driver(warm, f['env'])
        before = wait_usage(f['root'] / 'one-api.db', 16)
        time.sleep(.1)
        cmd = driver_command(args, f['direct'] if args.direct else f['url'], args.output / f'{trial}.json',
                             f'measure-{profile}-c{concurrency}-r{repeat}', concurrency, max(args.requests if profile == 'saturated' else args.paced_requests, concurrency * 2), chunks, pace)
        monitor = Resources(f['pids'])
        started = time.monotonic()
        try:
            report = run_driver(cmd, f['upstream_env'] if args.direct else f['env'], success=None if args.allow_overload else True)
            after = wait_usage(f['root'] / 'one-api.db', before[1] + (0 if args.direct else report['completed']))
            if not args.direct and report['completed'] and after[0] <= before[0]:
                raise AssertionError('successful traffic did not consume durable quota')
            settled_seconds = time.monotonic() - started
        finally:
            resources = monitor.finish(1, 1)  # Always join the sampling thread, including failing clients.
        for value in resources.values():
            value['cpu_ms_per_success'] = value['cpu_seconds'] * 1000 / report['completed'] if report['completed'] else None
            value['average_cores'] = value['cpu_seconds'] / settled_seconds
            value['measurement_seconds'] = settled_seconds
        driver_cpu = report['load_generator_cpu_seconds']
        resources['driver'] = {'cpu_seconds': driver_cpu, 'cpu_ms_per_success': driver_cpu * 1000 / report['completed'] if report['completed'] else None,
                               'average_cores': driver_cpu / report['seconds'], 'peak_rss_mib': None}
        report.update({'label': label, 'repeat': repeat, 'profile': profile, 'chunks': chunks,
                       'chunk_bytes': args.chunk_bytes, 'pace_ms': pace, 'rate': args.rate,
                       'binary_sha256': sha256(binary), 'resources': resources, 'direct': args.direct,
                       'settled_seconds': settled_seconds, 'settled_successful_rps': report['completed'] / settled_seconds,
                       'billing': {'requests': after[1] - before[1], 'used_quota': after[0] - before[0]}})
        (args.output / f'{trial}.json').write_text(json.dumps(report, indent=2) + '\n')
        print(json.dumps({key: report[key] for key in ('label', 'repeat', 'profile', 'concurrency', 'successful_rps', 'ttft_ms', 'total_ms', 'resources')}), flush=True)
        return {key: value for key, value in report.items() if key != 'samples'}


def checkpoint(summary: dict, output: Path) -> None:
    """checkpoint atomically saves progress so an interrupted experiment cannot masquerade as complete."""
    temporary = output / 'summary.json.tmp'
    temporary.write_text(json.dumps(summary, indent=2) + '\n')
    temporary.replace(output / 'summary.json')


def main() -> None:
    """main validates Linux inputs and alternates baseline/candidate order across repeated independent trials."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--baseline', type=Path)
    parser.add_argument('--driver', type=Path, default=Path(__file__).with_name('stream-perf'))
    parser.add_argument('--token-cache', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--concurrency', default='1,8,32,64')
    parser.add_argument('--profiles', default='saturated,paced')
    parser.add_argument('--repeats', type=int, default=3)
    parser.add_argument('--requests', type=int, default=128)
    parser.add_argument('--paced-requests', type=int, default=512)
    parser.add_argument('--chunks', type=int, default=1024)
    parser.add_argument('--chunk-bytes', type=int, default=128)
    parser.add_argument('--paced-chunks', type=int, default=32)
    parser.add_argument('--pace-ms', type=int, default=2)
    parser.add_argument('--rate', type=float, default=0)
    parser.add_argument('--allow-overload', action='store_true')
    parser.add_argument('--direct', action='store_true')
    parser.add_argument('--skip-qualification', action='store_true')
    parser.add_argument('--debug-logs', action='store_true')
    args = parser.parse_args()
    if platform.system() != 'Linux':
        parser.error('Linux /proc is required for target-process resource accounting')
    for name in ('binary', 'baseline', 'driver', 'token_cache', 'output'):
        value = getattr(args, name)
        if value is not None:
            setattr(args, name, value.resolve())
    levels = [int(value) for value in args.concurrency.split(',')]
    profiles = args.profiles.split(',')
    if not 1 <= args.repeats <= 20 or not 1 <= args.requests <= 1000000 or not 1 <= args.paced_requests <= 1000000 or any(c < 1 or c > 4096 for c in levels) or set(profiles) - {'saturated', 'paced'}:
        parser.error('invalid bounded test matrix')
    if args.output.exists() and any(args.output.iterdir()):
        parser.error('output directory must be empty; refusing to overwrite experiment evidence')
    args.output.mkdir(parents=True, exist_ok=True)
    variants = [('candidate', args.binary)] if args.baseline is None else [('baseline', args.baseline), ('candidate', args.binary)]
    summary = {'environment': {'platform': platform.platform(), 'cpu_count': os.cpu_count(),
                'cpu_affinity': sorted(os.sched_getaffinity(0)), 'cgroup_cpu_max': Path('/sys/fs/cgroup/cpu.max').read_text().strip() if Path('/sys/fs/cgroup/cpu.max').exists() else None,
                'GOMAXPROCS': 2, 'database': 'fresh SQLite; default WAL, quota, trace, logging, pool and billing settings',
                'cpu_model': next((line.split(':', 1)[1].strip() for line in Path('/proc/cpuinfo').read_text().splitlines() if line.startswith('model name')), 'unknown'),
                'memory_cgroup_max': Path('/sys/fs/cgroup/memory.max').read_text().strip() if Path('/sys/fs/cgroup/memory.max').exists() else None,
                'limiter': 'enabled; GLOBAL_RELAY_RATE_LIMIT and GLOBAL_API_RATE_LIMIT=10000000',
                'driver_sha256': sha256(args.driver), 'resource_sampling_ms': 20}, 'qualification': {}, 'trials': []}
    summary.update(schema_version=2, complete=False, started_at_utc=datetime.now(timezone.utc).isoformat(),
                   configuration={key: str(value) if isinstance(value, Path) else value for key, value in vars(args).items()})
    checkpoint(summary, args.output)
    try:
        for label, binary in variants:
            if not args.skip_qualification:
                summary['qualification'][label] = qualify(args, binary, label)
                checkpoint(summary, args.output)
        for repeat in range(args.repeats):
            for profile in profiles:
                for concurrency in levels:
                    for label, binary in variants[::1 if repeat % 2 == 0 else -1]:
                        summary['trials'].append(measure(args, binary, label, repeat, concurrency, profile))
                        checkpoint(summary, args.output)
        summary['complete'] = True
    finally:
        summary['finished_at_utc'] = datetime.now(timezone.utc).isoformat()
        checkpoint(summary, args.output)


if __name__ == '__main__':
    main()
