"""Verify browser-harness cleanup without requiring Playwright or a frontend build."""
import selectors
import signal
import subprocess
import sys
import unittest
from unittest.mock import Mock, call, patch

from browser_process import stop_process


class BrowserProcessTests(unittest.TestCase):
    """BrowserProcessTests cover normal shutdown, exit races and forced termination."""

    def test_graceful_shutdown_reaps_the_process(self) -> None:
        """test_graceful_shutdown_reaps_the_process requires a bounded wait after TERM."""
        process = Mock(pid=12345)
        with patch('browser_process.os.killpg') as kill:
            stop_process(process)
        kill.assert_called_once_with(12345, signal.SIGTERM)
        process.wait.assert_called_once_with(timeout=10.0)

    def test_already_exited_group_is_still_reaped(self) -> None:
        """test_already_exited_group_is_still_reaped treats a vanished group as normal."""
        process = Mock(pid=12345)
        with patch('browser_process.os.killpg', side_effect=ProcessLookupError):
            stop_process(process)
        process.wait.assert_called_once_with(timeout=10.0)

    def test_cleanup_preserves_the_original_failure(self) -> None:
        """test_cleanup_preserves_the_original_failure prevents cleanup from masking readiness errors."""
        process = Mock(pid=12345)
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
        process = Mock(pid=12345)
        process.wait.side_effect = [subprocess.TimeoutExpired('vite', 10), 0]
        with patch('browser_process.os.killpg') as kill:
            stop_process(process)
        self.assertEqual(kill.call_args_list, [call(12345, signal.SIGTERM), call(12345, signal.SIGKILL)])
        self.assertEqual(process.wait.call_args_list, [call(timeout=10.0), call(timeout=10.0)])

    def test_exit_between_timeout_and_kill_is_safe(self) -> None:
        """test_exit_between_timeout_and_kill_is_safe handles the second termination race too."""
        process = Mock(pid=12345)
        process.wait.side_effect = [subprocess.TimeoutExpired('vite', 10), 0]
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


if __name__ == '__main__':
    unittest.main(verbosity=2)
