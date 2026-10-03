"""Provision real loopback gateways and disposable SQLite fixtures without external credentials."""
from __future__ import annotations
import contextlib
import hashlib
import http.client
import json
import os
from pathlib import Path
import secrets
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time


def require(condition: bool, message: str) -> None:
    """require raises an invariant failure even when Python assertions are disabled."""
    if not condition:
        raise ValueError(message)


def sha256(path: Path) -> str:
    """sha256 hashes exact source or fixture bytes without loading a large file into memory."""
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def free_port() -> int:
    """free_port selects a free IPv4 loopback port; subsequent bind conflicts fail instead of killing listeners."""
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


def stop(process: subprocess.Popen) -> None:
    """stop terminates and reaps only an owned gateway with a bounded fallback."""
    if process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)


def get(conn: http.client.HTTPConnection, path: str, token: str = '') -> tuple[int, dict, float, int]:
    """get measures a complete bounded HTTP body before JSON/oracle work; credentials never enter diagnostics."""
    require(path.startswith('/api/') and '\r' not in path and '\n' not in path, 'invalid local API path')
    started = time.perf_counter_ns()
    conn.request('GET', path, headers={'Authorization': 'Bearer '+token, 'Accept': 'application/json'})
    response = conn.getresponse()
    content = response.read((16 << 20) + 1)
    elapsed = (time.perf_counter_ns() - started) / 1e6
    require(len(content) <= 16 << 20, 'HTTP response exceeded 16 MiB')
    require(response.getheader('Content-Type', '').startswith('application/json'), 'response is not JSON')
    return response.status, json.loads(content), elapsed, len(content)


@contextlib.contextmanager
def gateway(binary: Path, directory: Path, *, cursor: bool = True, pprof_port: int | None = None, gateway_procs: int = 2):
    """gateway starts a normal isolated process and guarantees cleanup without exporting its private log or database."""
    require(type(gateway_procs) is int and 1 <= gateway_procs <= 64, 'invalid gateway CPU slots')
    require(pprof_port is None or type(pprof_port) is int and 1 <= pprof_port <= 65535, 'invalid pprof port')
    check_cache(Path(os.environ.get('TIKTOKEN_CACHE_DIR', '')))
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    port, admin = free_port(), secrets.token_hex(24)
    env = {'PATH': os.environ.get('PATH', '/usr/bin:/bin'), 'HOME': str(directory), 'TZ': 'UTC',
           'GOMAXPROCS': str(gateway_procs), 'GIN_MODE': 'release', 'PORT': str(port), 'LISTEN_HOST': '127.0.0.1',
           'SQLITE_PATH': str(directory/'one-api.db'), 'INITIAL_ROOT_ACCESS_TOKEN': admin,
           'GLOBAL_API_RATE_LIMIT': '10000000', 'TIKTOKEN_CACHE_DIR': os.environ.get('TIKTOKEN_CACHE_DIR', ''), 'LOG_CURSOR_ENABLED': str(cursor).lower()}
    if pprof_port:
        env.update(ENABLE_PPROF='true', PPROF_LISTEN=f'127.0.0.1:{pprof_port}')
    start = time.monotonic()
    with (directory/'private-gateway.log').open('w') as log:
        process = subprocess.Popen([str(binary.resolve())], cwd=directory, env=env, stdout=log, stderr=subprocess.STDOUT)
        try:
            deadline = start + 90
            while time.monotonic() < deadline:
                require(process.poll() is None, 'fixture gateway exited before readiness')
                conn = http.client.HTTPConnection('127.0.0.1', port, timeout=2)
                try:
                    status, body, _, _ = get(conn, '/api/status')
                    if status == 200 and body.get('success') is True:
                        break
                except (OSError, http.client.HTTPException):
                    time.sleep(.05)
                finally:
                    conn.close()
            else:
                raise TimeoutError('gateway readiness timeout')
            yield {'pid': process.pid, 'port': port, 'bootstrap_token': admin,
                   'startup_seconds': time.monotonic()-start, 'database': directory/'one-api.db'}
        finally:
            stop(process)


def bootstrap(binary: Path, directory: Path) -> Path:
    """bootstrap obtains the actual migrated schema from the gateway before deterministic seeding."""
    with gateway(binary, directory):
        pass
    return directory/'one-api.db'


def clone_database(source: Path, destination: Path) -> None:
    """clone_database creates a consistent private copy including committed WAL state, without copying secret logs."""
    with sqlite3.connect(source) as src, sqlite3.connect(destination) as dst:
        src.backup(dst)


def check_cache(directory: Path) -> None:
    """check_cache calls the existing pinned read-only validator before any gateway can try an implicit download."""
    script = Path(__file__).resolve().parents[1] / 'stream-perf/cache_tokens.py'
    subprocess.run([sys.executable, str(script), '--cache', str(directory), '--check-only'],
                   check=True, capture_output=True, timeout=30)
