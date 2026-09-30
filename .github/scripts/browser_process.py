"""Bounded POSIX process cleanup for the browser acceptance harness."""
import contextlib
import os
import signal
import subprocess


def stop_process(process: subprocess.Popen, timeout: float = 10.0) -> None:
    """stop_process kills process's owned group and reaps its leader, returning None.

    The caller must launch process with start_new_session=True. Each leader
    wait is bounded by timeout seconds; descendants that remain in the owned
    group receive SIGKILL even when the leader has already exited.
    """
    if timeout <= 0:
        raise ValueError('The cleanup timeout must be positive')
    with contextlib.suppress(ProcessLookupError):
        os.killpg(process.pid, signal.SIGTERM)
    try:
        process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        pass  # The forced group termination below handles a slow leader too.

    # wait() covers only the leader: a surviving descendant may ignore TERM.
    # Signal the owned group even after a successful wait or cached leader exit.
    with contextlib.suppress(ProcessLookupError):
        os.killpg(process.pid, signal.SIGKILL)
    if process.poll() is None:
        process.wait(timeout=timeout)
