"""Bounded POSIX browser-process cleanup with lossless failure reporting (Python 3.11+)."""
import contextlib
import math
import os
import signal
import subprocess
from collections.abc import Iterator


def stop_process(process: subprocess.Popen, timeout: float = 10.0) -> None:
    """stop_process terminates process's owned group and reaps its leader, returning None.

    Launch process with start_new_session=True. Each of at most two leader
    waits uses the finite, positive timeout in seconds. Every cleanup phase is
    attempted even after an interruption or unexpected signal/wait failure.
    Single failures retain their identity; multiple failures are grouped, so
    cancellation, permission errors, and final timeouts are never swallowed.
    Descendants must remain in the owned group; escaped sessions are not owned.
    """
    if not math.isfinite(timeout) or timeout <= 0:
        raise ValueError('The cleanup timeout must be finite and positive')
    failures: list[BaseException] = []
    try:
        with contextlib.suppress(ProcessLookupError):
            os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            pass  # A slow leader still needs the forced phase below.
    except BaseException as error:
        # Defer propagation only until the remaining bounded cleanup attempts.
        failures.append(error)

    # wait() covers only the leader. Always signal surviving group members,
    # including after a successful wait, cached exit, or interrupted wait.
    try:
        with contextlib.suppress(ProcessLookupError):
            os.killpg(process.pid, signal.SIGKILL)
    except BaseException as error:
        failures.append(error)
    try:
        if process.poll() is None:
            process.wait(timeout=timeout)
    except BaseException as error:
        failures.append(error)

    if len(failures) == 1:
        raise failures[0]
    if failures:
        raise BaseExceptionGroup('Multiple browser-process cleanup failures', failures)


@contextlib.contextmanager
def process_cleanup(process: subprocess.Popen, timeout: float = 10.0) -> Iterator[subprocess.Popen]:
    """process_cleanup yields process and cleans it once on success, failure, or cancellation.

    The owned process and timeout follow stop_process's contract. A task error
    is re-raised unchanged when cleanup succeeds. If both fail, a group retains
    the original task exception first and the cleanup exception second instead
    of allowing cleanup to replace the task failure. This requires Python 3.11+.
    """
    try:
        yield process
    except BaseException as original:
        try:
            stop_process(process, timeout=timeout)
        except BaseException as cleanup:
            raise BaseExceptionGroup('Browser task and cleanup both failed', [original, cleanup]) from None
        raise
    else:
        stop_process(process, timeout=timeout)
