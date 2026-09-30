"""Bounded POSIX process cleanup for the browser acceptance harness."""
import contextlib
import os
import signal
import subprocess


def stop_process(process: subprocess.Popen, timeout: float = 10.0) -> None:
    """stop_process terminates the owned group and reaps its leader despite exit races."""
    if timeout <= 0:
        raise ValueError('The cleanup timeout must be positive')
    with contextlib.suppress(ProcessLookupError):
        os.killpg(process.pid, signal.SIGTERM)
    try:
        process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        with contextlib.suppress(ProcessLookupError):
            os.killpg(process.pid, signal.SIGKILL)
        process.wait(timeout=timeout)
