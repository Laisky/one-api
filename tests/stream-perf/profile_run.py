#!/usr/bin/env python3
"""Run bounded loopback streaming diagnostics with independently sampled gateway CPU and RSS."""
from __future__ import annotations
import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import platform
import subprocess
import time

import cache_tokens
import run
from profile_capture import capture
from profile_support import assert_loopback_listeners, cpu_allowance, snapshot, stable_window, write_json


def diagnose(args: argparse.Namespace) -> dict:
    """diagnose qualifies a fresh gateway, samples a fixed workload and retains failed stability/correctness results."""
    args.output.mkdir(parents=True, exist_ok=False)
    allowance = cpu_allowance()
    summary = {'complete': False, 'diagnostic_only': True, 'started_utc': datetime.now(timezone.utc).isoformat(),
               'configuration': {k: str(v) if isinstance(v, Path) else v for k, v in vars(args).items()},
               'allowance': allowance, 'platform': platform.platform(),
               'cpu_model': next(line.split(':', 1)[1].strip() for line in Path('/proc/cpuinfo').read_text().splitlines()
                                 if line.startswith('model name')),
               'binary_sha256': run.sha256(args.binary), 'driver_sha256': run.sha256(args.driver),
               'profile_captures': [], 'admission_rule_version': 2}
    write_json(args.output / 'summary.json', summary)
    try:
        summary['qualification'] = run.qualify(args, args.binary, 'diagnostic', gateway_procs=args.gateway_procs,
                                              auxiliary_procs=args.auxiliary_procs, client_procs=getattr(args, 'client_procs', None))
        pprof_port = run.free_port() if args.mode in ('cpu', 'heap', 'trace') else None
        with run.fixture(args, args.binary, gateway_procs=args.gateway_procs,
                         auxiliary_procs=args.auxiliary_procs, pprof_port=pprof_port,
                         client_procs=getattr(args, 'client_procs', None)) as fixture:
            summary['process_slots'] = fixture['process_slots']
            ports = {int(fixture['url'].split(':')[2].split('/')[0])}
            if pprof_port is not None:
                ports.add(pprof_port)
            assert_loopback_listeners(fixture['pids']['gateway'], ports)
            database = fixture['root'] / 'one-api.db'
            warm = run.driver_command(args, fixture['url'], args.output / 'initial-warmup.json', 'warmup', 8, 16, 8, 0)
            run.run_driver(warm, fixture['env'])
            before = run.wait_usage(database, 16)
            command = run.driver_command(args, fixture['url'], args.output / 'requests.json', 'steady', args.concurrency,
                                         args.requests, args.chunks, args.pace_ms)
            log = args.output / 'driver.log'
            with log.open('w') as output:
                client = subprocess.Popen(command, env=fixture['env'], stdout=output, stderr=subprocess.STDOUT)
                try:
                    started = time.monotonic()
                    pids = {**fixture['pids'], 'driver': client.pid}
                    rows, profile_started, profile_finished = [], None, False
                    future = None
                    trace_started = False
                    next_sample = started
                    with ThreadPoolExecutor(max_workers=1) as profiler, (args.output / 'samples.jsonl').open('w') as samples:
                        while client.poll() is None:
                            elapsed = time.monotonic() - started
                            if elapsed > args.deadline:
                                raise TimeoutError('bounded diagnostic workload exceeded its deadline')
                            if elapsed >= args.warmup and profile_started is None:
                                profile_started = elapsed
                                if args.mode == 'cpu':
                                    future = profiler.submit(capture, fixture['pprof_url'] + f'/profile?seconds={args.seconds}',
                                                             args.output / 'cpu.pprof', args.seconds + 15)
                                elif args.mode == 'heap':
                                    summary['profile_captures'].append(capture(
                                        fixture['pprof_url'] + '/heap?gc=1', args.output / 'heap-before.pprof', 15))
                            if (args.mode == 'trace' and profile_started is not None and not trace_started
                                    and elapsed >= profile_started + args.trace_offset):
                                trace_started = True
                                future = profiler.submit(capture, fixture['pprof_url'] + f'/trace?seconds={args.trace_seconds}',
                                                         args.output / 'runtime.trace', args.trace_seconds + 15)
                            if profile_started is not None and elapsed >= profile_started + args.seconds and not profile_finished:
                                if future is not None:
                                    summary['profile_captures'].append(future.result(timeout=15))
                                    future = None
                                elif args.mode == 'heap':
                                    summary['profile_captures'].append(capture(
                                        fixture['pprof_url'] + '/heap?gc=1', args.output / 'heap-after.pprof', 15))
                                profile_finished = client.poll() is None
                            try:
                                row = snapshot(pids, fixture['mock_url'], database, Path(allowance['stat_path']), started)
                            except FileNotFoundError:
                                if client.poll() is None:
                                    raise
                                break
                            rows.append(row)
                            samples.write(json.dumps(row, allow_nan=False) + '\n')
                            samples.flush()
                            next_sample += 1
                            time.sleep(max(0, next_sample - time.monotonic()))
                        if future is not None:
                            summary['profile_captures'].append(future.result(timeout=args.seconds + 15))
                    for captured in summary['profile_captures']:
                        captured['started_elapsed'] = captured.pop('started_monotonic') - started
                        captured['finished_elapsed'] = captured.pop('finished_monotonic') - started
                    if args.mode == 'trace':
                        summary['trace_note'] = ('A bounded subwindow only; not a full-observation trace. '
                                                 'Capture wall time includes HTTP overhead, not just tracer-active time.')
                    report = json.loads((args.output / 'requests.json').read_text())
                    summary['requests'] = {k: v for k, v in report.items() if k != 'samples'}
                    if client.wait(timeout=5) or report['failed'] or report['dropped'] or report['completed'] != args.requests:
                        raise AssertionError('diagnostic workload failed exact delivery')
                    after = run.wait_usage(database, before[1] + report['completed'])
                    summary['usage'] = {'requests': after[1] - before[1], 'used_quota': after[0] - before[0]}
                    if summary['usage']['used_quota'] <= 0:
                        raise AssertionError('durable usage did not increase')
                    summary['window'] = stable_window(rows, allowance['effective_cores'], args.gateway_procs,
                                                      profile_started or args.warmup, args.seconds)
                    trace_valid = True
                    if args.mode == 'trace':
                        captures = summary['profile_captures']
                        trace_valid = (len(captures) == 1 and profile_started is not None
                                       and captures[0]['started_elapsed'] >= profile_started
                                       and captures[0]['finished_elapsed'] <= profile_started + args.seconds)
                        summary['trace_window_within_observation'] = trace_valid
                    summary['profile_window_completed_while_load_alive'] = profile_finished
                    summary['sustained_protocol_qualified'] = (summary['window']['qualified'] and profile_finished and trace_valid
                                                             and args.warmup >= 30 and args.seconds >= 60)
                    summary['complete'] = True
                finally:
                    run.stop(client)
    except Exception as error:
        summary['failure_type'] = type(error).__name__
        if isinstance(error, run.BillingMismatch):
            summary['failure_usage'] = error.evidence
        raise
    finally:
        summary['finished_utc'] = datetime.now(timezone.utc).isoformat()
        write_json(args.output / 'summary.json', summary)
    return summary


def main() -> None:
    """main validates bounded diagnostic inputs and refuses existing output or implicit tokenizer downloads."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--driver', type=Path, required=True)
    parser.add_argument('--token-cache', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--mode', choices=('calibration', 'steady', 'cpu', 'heap', 'trace'), required=True)
    parser.add_argument('--gateway-procs', type=int, default=4)
    parser.add_argument('--auxiliary-procs', type=int, default=1)
    parser.add_argument('--client-procs', type=int, default=None,
                        help='explicit load-client slots; default uses auxiliary-procs, without changing mock slots')
    parser.add_argument('--concurrency', type=int, default=32)
    parser.add_argument('--requests', type=int, default=10000)
    parser.add_argument('--chunks', type=int, default=1024)
    parser.add_argument('--chunk-bytes', type=int, default=128)
    parser.add_argument('--pace-ms', type=int, default=0)
    parser.add_argument('--warmup', type=int, default=30)
    parser.add_argument('--seconds', type=int, default=60)
    parser.add_argument('--deadline', type=int, default=600)
    parser.add_argument('--trace-seconds', type=int, default=5)
    parser.add_argument('--trace-offset', type=int, default=15)
    args = parser.parse_args()
    if platform.system() != 'Linux':
        parser.error('Linux is required')
    if not (1 <= args.gateway_procs <= 64 and 1 <= args.auxiliary_procs <= 64 and 1 <= args.concurrency <= 4096
            and 1 <= args.requests <= 1000000 and 1 <= args.chunks <= 16384 and 96 <= args.chunk_bytes <= 16384
            and args.chunks * args.chunk_bytes <= 64 << 20 and 0 <= args.pace_ms <= 1000
            and 1 <= args.warmup <= 300 and 1 <= args.seconds <= 300 and args.warmup + args.seconds < args.deadline <= 1200):
        parser.error('invalid bounded diagnostic configuration')
    if args.client_procs is not None and not 1 <= args.client_procs <= 64:
        parser.error('client-procs must be between 1 and 64')
    if args.mode == 'trace' and not (1 <= args.trace_seconds <= 10 and 0 <= args.trace_offset
            and args.trace_offset + args.trace_seconds <= args.seconds):
        parser.error('trace subwindow must be 1-10 seconds within the observation')
    if args.mode != 'calibration' and (args.warmup < 30 or args.seconds < 60):
        parser.error('sustained diagnostics require at least 30s warmup and 60s observation')
    for name in ('binary', 'driver', 'token_cache', 'output'):
        setattr(args, name, getattr(args, name).resolve())
    if args.output.exists():
        parser.error('refusing to overwrite diagnostic evidence')
    # Use the same read-only asset check as normal offline comparisons.
    subprocess.run([os.sys.executable, str(Path(__file__).with_name('cache_tokens.py')), '--cache', str(args.token_cache),
                    '--check-only'], check=True, timeout=30)
    args.rate, args.debug_logs = 0, False
    summary = diagnose(args)
    print(json.dumps({'complete': summary['complete'], 'qualified': summary['sustained_protocol_qualified'],
                      'window': {k: v for k, v in summary['window'].items() if k != 'intervals'}}))
    if args.mode != 'calibration' and not summary['sustained_protocol_qualified']:
        raise SystemExit('Sustained-load gate was not met; diagnostic evidence is retained without an acceptance claim.')


if __name__ == '__main__':
    main()
