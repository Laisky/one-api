"""Verify browser-harness cleanup without requiring Playwright or a frontend build."""
import contextlib
import ctypes
import os
import selectors
import signal
import subprocess
import sys
import textwrap
import time
import unittest
from unittest.mock import Mock, call, patch

from browser_process import stop_process
# unittest.main also discovers this imported suite; browser CI requires both.
from test_browser_cleanup_failures import BrowserCleanupFailureTests


class BrowserProcessTests(unittest.TestCase):
    """BrowserProcessTests cover normal shutdown, exit races and forced termination."""

    def test_graceful_shutdown_reaps_the_process(self) -> None:
        """test_graceful_shutdown_reaps_the_process requires a bounded wait after TERM."""
        process = Mock(pid=12345, poll=Mock(return_value=0))
        with patch('browser_process.os.killpg') as kill:
            stop_process(process)
        self.assertEqual(kill.call_args_list, [call(12345, signal.SIGTERM), call(12345, signal.SIGKILL)])
        process.wait.assert_called_once_with(timeout=10.0)

    def test_already_exited_group_is_still_reaped(self) -> None:
        """test_already_exited_group_is_still_reaped treats a vanished group as normal."""
        process = Mock(pid=12345, poll=Mock(return_value=0))
        with patch('browser_process.os.killpg', side_effect=ProcessLookupError):
            stop_process(process)
        process.wait.assert_called_once_with(timeout=10.0)

    def test_cleanup_preserves_the_original_failure(self) -> None:
        """test_cleanup_preserves_the_original_failure prevents cleanup from masking readiness errors."""
        process = Mock(pid=12345, poll=Mock(return_value=0))
        original = RuntimeError('Vite exited before readiness')
        with patch('browser_process.os.killpg', side_effect=ProcessLookupError):
            with self.assertRaises(RuntimeError) as caught:
                try:
                    raise original
                finally:
                    stop_process(process)
        self.assertIs(caught.exception, original)
        process.wait.assert_called_once_with(timeout=10.0)

    def test_timeout_escalates_and_reaps(self) -> None:
        """test_timeout_escalates_and_reaps kills a process that ignores graceful termination."""
        process = Mock(pid=12345, poll=Mock(return_value=0))
        process.wait.side_effect = [subprocess.TimeoutExpired('vite', 10), 0]
        process.poll.return_value = None
        with patch('browser_process.os.killpg') as kill:
            stop_process(process)
        self.assertEqual(kill.call_args_list, [call(12345, signal.SIGTERM), call(12345, signal.SIGKILL)])
        self.assertEqual(process.wait.call_args_list, [call(timeout=10.0), call(timeout=10.0)])

    def test_exit_between_timeout_and_kill_is_safe(self) -> None:
        """test_exit_between_timeout_and_kill_is_safe handles the second termination race too."""
        process = Mock(pid=12345, poll=Mock(return_value=0))
        process.wait.side_effect = [subprocess.TimeoutExpired('vite', 10), 0]
        process.poll.return_value = None
        with patch('browser_process.os.killpg', side_effect=[None, ProcessLookupError]):
            stop_process(process)
        self.assertEqual(process.wait.call_count, 2)

    def test_real_already_exited_process(self) -> None:
        """test_real_already_exited_process verifies cleanup against an actual reaped child."""
        with subprocess.Popen([sys.executable, '-c', 'pass'], start_new_session=True) as process:
            process.wait(timeout=5)
            stop_process(process, timeout=1)
            self.assertEqual(process.returncode, 0)

    def test_real_process_ignoring_sigterm(self) -> None:
        """test_real_process_ignoring_sigterm verifies forced cleanup and child reaping."""
        script = "import signal,time; signal.signal(signal.SIGTERM, signal.SIG_IGN); print('ready',flush=True); time.sleep(60)"
        with subprocess.Popen([sys.executable, '-u', '-c', script], start_new_session=True, stdout=subprocess.PIPE, text=True) as process:
            try:
                with selectors.DefaultSelector() as selector:
                    selector.register(process.stdout, selectors.EVENT_READ)
                    self.assertTrue(selector.select(timeout=5), 'The child must announce readiness')
                self.assertEqual(process.stdout.readline().strip(), 'ready')
                stop_process(process, timeout=1)
                self.assertEqual(process.returncode, -signal.SIGKILL)
            finally:
                if process.poll() is None:
                    process.kill()
                process.wait(timeout=5)


    def test_leader_exit_after_timeout_needs_no_second_wait(self) -> None:
        """test_leader_exit_after_timeout_needs_no_second_wait verifies poll reaping after a race."""
        process = Mock(pid=12345, poll=Mock(return_value=0))
        process.wait.side_effect = subprocess.TimeoutExpired('vite', 10)
        with patch('browser_process.os.killpg', side_effect=[None, ProcessLookupError]):
            stop_process(process)
        process.wait.assert_called_once_with(timeout=10.0)
        process.poll.assert_called_once_with()

    def test_forced_wait_remains_bounded(self) -> None:
        """test_forced_wait_remains_bounded requires an unsuccessful final wait to propagate."""
        process = Mock(pid=12345, poll=Mock(return_value=None))
        original = subprocess.TimeoutExpired('vite', 0.1)
        process.wait.side_effect = original
        with patch('browser_process.os.killpg') as kill:
            with self.assertRaises(subprocess.TimeoutExpired) as caught:
                stop_process(process, timeout=0.1)
        self.assertIs(caught.exception, original)
        self.assertEqual(process.wait.call_args_list, [call(timeout=0.1), call(timeout=0.1)])
        self.assertEqual(kill.call_args_list, [call(12345, signal.SIGTERM), call(12345, signal.SIGKILL)])

    def test_unexpected_signal_errors_are_not_suppressed(self) -> None:
        """test_unexpected_signal_errors_are_not_suppressed checks permission errors at both signals."""
        for failures in ([PermissionError('denied'), None], [None, PermissionError('denied')]):
            with self.subTest(failed_signal='TERM' if failures[0] is not None else 'KILL'):
                process = Mock(pid=12345, poll=Mock(return_value=0))
                with patch('browser_process.os.killpg', side_effect=failures):
                    with self.assertRaises(PermissionError):
                        stop_process(process)

    def test_invalid_timeout_does_not_signal(self) -> None:
        """test_invalid_timeout_does_not_signal rejects nonpositive timeouts before side effects."""
        for timeout in (0, -1):
            with self.subTest(timeout=timeout):
                process = Mock(pid=12345)
                with patch('browser_process.os.killpg') as kill:
                    with self.assertRaises(ValueError):
                        stop_process(process, timeout=timeout)
                kill.assert_not_called()
                process.wait.assert_not_called()

    @unittest.skipUnless(sys.platform == 'linux', 'Orphan reaping uses the Linux CI subreaper API')
    def test_real_descendant_survives_graceful_leader_exit(self) -> None:
        """test_real_descendant_survives_graceful_leader_exit requires killing the surviving child."""
        self._check_descendant_cleanup(exit_before_cleanup=False)

    @unittest.skipUnless(sys.platform == 'linux', 'Orphan reaping uses the Linux CI subreaper API')
    def test_real_descendant_survives_already_exited_leader(self) -> None:
        """test_real_descendant_survives_already_exited_leader covers cached leader exit status."""
        self._check_descendant_cleanup(exit_before_cleanup=True)

    def _check_descendant_cleanup(self, exit_before_cleanup: bool) -> None:
        """_check_descendant_cleanup checks the chosen leader exit mode and reaps both real processes."""
        # Adopt the orphan only inside this standalone test process, so neither
        # a failing negative control nor a slow container PID 1 leaves zombies.
        libc = ctypes.CDLL(None, use_errno=True)
        previous = ctypes.c_int()
        self.assertEqual(libc.prctl(37, ctypes.byref(previous), 0, 0, 0), 0)  # PR_GET_CHILD_SUBREAPER
        self.assertEqual(libc.prctl(36, 1, 0, 0, 0), 0)  # PR_SET_CHILD_SUBREAPER
        descendant = None
        reaped = False
        child_script = (
            "import os,signal,time; signal.signal(signal.SIGTERM, signal.SIG_IGN); "
            "print(os.getpid(),os.getppid(),os.getpgrp(),flush=True); time.sleep(60)"
        )
        leader_script = textwrap.dedent(f"""
            import signal, subprocess, sys
            def exit_cleanly(_signal, _frame):
                '''exit_cleanly exits the fixture leader successfully on SIGTERM.'''
                sys.exit(0)
            signal.signal(signal.SIGTERM, exit_cleanly)
            subprocess.Popen([sys.executable, '-u', '-c', {child_script!r}])
            sys.stdin.readline()
        """)
        try:
            with subprocess.Popen(
                [sys.executable, '-u', '-c', leader_script],
                start_new_session=True, stdin=subprocess.PIPE,
                stdout=subprocess.PIPE, text=True,
            ) as process:
                try:
                    with selectors.DefaultSelector() as selector:
                        selector.register(process.stdout, selectors.EVENT_READ)
                        self.assertTrue(selector.select(timeout=5), 'The descendant must announce readiness')
                    descendant, parent, group = map(int, process.stdout.readline().split())
                    self.assertEqual(parent, process.pid, 'The fixture must be a real descendant')
                    self.assertEqual(group, process.pid, 'The descendant must share the owned group')
                    if exit_before_cleanup:
                        process.stdin.write('exit\n')
                        process.stdin.flush()
                        self.assertEqual(process.wait(timeout=5), 0)
                    # Allow scheduler/interpreter shutdown latency under CI load.
                    # The mocked timeout cases separately enforce bounded waits.
                    stop_process(process, timeout=5)
                    self.assertEqual(process.returncode, 0, 'The leader must exit gracefully, not time out')
                    status = self._wait_for_descendant(descendant, timeout=2)
                    self.assertIsNotNone(status, 'Cleanup left the SIGTERM-ignoring descendant alive')
                    reaped = True
                    self.assertEqual(os.waitstatus_to_exitcode(status), -signal.SIGKILL)
                    with self.assertRaises(ProcessLookupError):
                        os.killpg(process.pid, 0)
                finally:
                    # Independent cleanup is essential: the old helper is
                    # deliberately run here as a negative control.
                    with contextlib.suppress(ProcessLookupError):
                        os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=5)
                    if descendant is not None and not reaped:
                        status = self._wait_for_descendant(descendant, timeout=5)
                        self.assertIsNotNone(status, 'The fixture must reap its orphan even on failure')
                    # Handle a startup failure before the PID handshake too.
                    if descendant is None:
                        deadline = time.monotonic() + 5
                        while time.monotonic() < deadline:
                            try:
                                pid, _ = os.waitpid(-process.pid, os.WNOHANG)
                            except ChildProcessError:
                                break
                            if pid == 0:
                                time.sleep(0.01)
                        else:
                            self.fail('The fixture failed to reap its process group')
        finally:
            self.assertEqual(libc.prctl(36, previous.value, 0, 0, 0), 0)

    def _wait_for_descendant(self, pid: int, timeout: float) -> int | None:
        """_wait_for_descendant returns the adopted PID's wait status or None after timeout seconds."""
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            found, status = os.waitpid(pid, os.WNOHANG)
            if found == pid:
                return status
            time.sleep(0.01)
        return None


if __name__ == '__main__':
    unittest.main(verbosity=2)
