"""Explicit secondary trace capture for a separately built, instrumented load driver."""
from __future__ import annotations
from pathlib import Path

from profile_capture import capture
from profile_support import assert_loopback_listeners
import run

ENVIRONMENT = 'STREAM_PERF_CLIENT_TRACE_LISTEN'


def configuration(args, environment: dict) -> tuple[dict, int | None]:
    """configuration copies the driver environment and binds a client listener only on explicit trace requests."""
    enabled = getattr(args, 'client_trace', False)
    if type(enabled) is not bool or enabled and args.mode != 'trace':
        raise ValueError('client trace requires explicit trace mode')
    result = dict(environment)
    # Never inherit a listener from the caller, even if a future fixture propagates more environment.
    result.pop(ENVIRONMENT, None)
    if not enabled:
        return result, None
    port = run.free_port()
    result[ENVIRONMENT] = f'127.0.0.1:{port}'
    return result, port


def collect(pid: int, port: int, seconds: int, output: Path) -> dict:
    """collect verifies the driver's real listening descriptors then uses the existing bounded collector."""
    if type(port) is not int or not 1 <= port <= 65535 or type(seconds) is not int or not 1 <= seconds <= 10:
        raise ValueError('invalid bounded client capture')
    assert_loopback_listeners(pid, {port})
    captured = capture(f'http://127.0.0.1:{port}/debug/pprof/trace?seconds={seconds}', output, seconds + 15)
    return {**captured, 'process_role': 'driver', 'pid': pid, 'requested_seconds': seconds}


def inside(captured: dict | None, started: float, window_start: float | None, duration: float) -> bool:
    """inside requires a complete client trace wholly inside the same declared load window."""
    if captured is None or window_start is None:
        return False
    begin = captured['started_monotonic'] - started
    end = captured['finished_monotonic'] - started
    return captured.get('process_role') == 'driver' and window_start <= begin < end <= window_start + duration
