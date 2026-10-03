"""Exercise interruption and multiple-failure cleanup without hiding either failure."""
import ast
import contextlib
from pathlib import Path
import selectors
import signal
import subprocess
import sys
import unittest
from unittest.mock import Mock, call, patch

import browser_process


class BrowserCleanupFailureTests(unittest.TestCase):
    """BrowserCleanupFailureTests require bounded cleanup after failures and cancellation."""

    def test_interrupted_wait_still_kills_and_reaps(self) -> None:
        """test_interrupted_wait_still_kills_and_reaps retains cancellation after forced cleanup."""
        process = Mock(pid=12345, poll=Mock(return_value=None))
        original = KeyboardInterrupt('interrupted graceful wait')
        process.wait.side_effect = [original, 0]
        with patch('browser_process.os.killpg') as kill:
            with self.assertRaises(KeyboardInterrupt) as caught:
                browser_process.stop_process(process, timeout=0.1)
        self.assertIs(caught.exception, original)
        self.assertEqual(kill.call_args_list, [call(12345, signal.SIGTERM), call(12345, signal.SIGKILL)])
        self.assertEqual(process.wait.call_args_list, [call(timeout=0.1), call(timeout=0.1)])

    def test_term_failure_still_attempts_kill_and_reaping(self) -> None:
        """test_term_failure_still_attempts_kill_and_reaping reports the original permission error."""
        process = Mock(pid=12345, poll=Mock(return_value=None))
        original = PermissionError('TERM denied')
        with patch('browser_process.os.killpg', side_effect=[original, None]) as kill:
            with self.assertRaises(PermissionError) as caught:
                browser_process.stop_process(process, timeout=0.1)
        self.assertIs(caught.exception, original)
        self.assertEqual(kill.call_args_list, [call(12345, signal.SIGTERM), call(12345, signal.SIGKILL)])
        process.wait.assert_called_once_with(timeout=0.1)

    def test_kill_failure_does_not_skip_reaping(self) -> None:
        """test_kill_failure_does_not_skip_reaping still gives the leader one bounded wait."""
        process = Mock(pid=12345, poll=Mock(return_value=None))
        process.wait.side_effect = [subprocess.TimeoutExpired('vite', 0.1), 0]
        original = PermissionError('KILL denied')
        with patch('browser_process.os.killpg', side_effect=[None, original]):
            with self.assertRaises(PermissionError) as caught:
                browser_process.stop_process(process, timeout=0.1)
        self.assertIs(caught.exception, original)
        self.assertEqual(process.wait.call_args_list, [call(timeout=0.1), call(timeout=0.1)])

    def test_multiple_cleanup_failures_retain_all_causes(self) -> None:
        """test_multiple_cleanup_failures_retain_all_causes preserves interruption and later errors."""
        process = Mock(pid=12345, poll=Mock(return_value=None))
        interrupted = KeyboardInterrupt('interrupted')
        denied = PermissionError('KILL denied')
        expired = subprocess.TimeoutExpired('vite', 0.1)
        process.wait.side_effect = [interrupted, expired]
        with patch('browser_process.os.killpg', side_effect=[None, denied]):
            with self.assertRaises(BaseExceptionGroup) as caught:
                browser_process.stop_process(process, timeout=0.1)
        self.assertEqual(caught.exception.exceptions, (interrupted, denied, expired))
        self.assertEqual(process.wait.call_args_list, [call(timeout=0.1), call(timeout=0.1)])

    def test_nonfinite_timeout_has_no_side_effects(self) -> None:
        """test_nonfinite_timeout_has_no_side_effects rejects budgets that cannot bound cleanup."""
        for timeout in (float('inf'), float('-inf'), float('nan')):
            with self.subTest(timeout=timeout):
                process = Mock(pid=12345)
                with patch('browser_process.os.killpg') as kill:
                    with self.assertRaises(ValueError):
                        browser_process.stop_process(process, timeout=timeout)
                kill.assert_not_called()
                process.wait.assert_not_called()

    def test_scope_keeps_primary_failure_when_cleanup_succeeds(self) -> None:
        """test_scope_keeps_primary_failure_when_cleanup_succeeds propagates the same task exception."""
        process = Mock(pid=12345)
        original = RuntimeError('Vite exited before readiness')
        with patch('browser_process.stop_process') as stop:
            with self.assertRaises(RuntimeError) as caught:
                with browser_process.process_cleanup(process, timeout=0.1):
                    raise original
        self.assertIs(caught.exception, original)
        stop.assert_called_once_with(process, timeout=0.1)

    def test_scope_reports_task_and_cleanup_failures(self) -> None:
        """test_scope_reports_task_and_cleanup_failures keeps task and cleanup exceptions in order."""
        for original in (RuntimeError('browser failed'), KeyboardInterrupt('cancelled')):
            with self.subTest(primary=type(original).__name__):
                process = Mock(pid=12345)
                failure = PermissionError('cleanup failed')
                with patch('browser_process.stop_process', side_effect=failure):
                    with self.assertRaises(BaseExceptionGroup) as caught:
                        with browser_process.process_cleanup(process, timeout=0.1):
                            raise original
                self.assertEqual(caught.exception.exceptions, (original, failure))

    def test_scope_reports_cleanup_only_failure(self) -> None:
        """test_scope_reports_cleanup_only_failure rejects a successful task with failed cleanup."""
        failure = PermissionError('cleanup failed')
        with patch('browser_process.stop_process', side_effect=failure):
            with self.assertRaises(PermissionError) as caught:
                with browser_process.process_cleanup(Mock(pid=12345)):
                    pass
        self.assertIs(caught.exception, failure)

    def test_scope_yields_owned_process_and_cleans_once(self) -> None:
        """test_scope_yields_owned_process_and_cleans_once preserves success and calls cleanup once."""
        process = Mock(pid=12345)
        with patch('browser_process.stop_process') as stop:
            with browser_process.process_cleanup(process, timeout=0.1) as owned:
                self.assertIs(owned, process)
        stop.assert_called_once_with(process, timeout=0.1)

    def test_browser_entrypoint_owns_cleanup_scope(self) -> None:
        """test_browser_entrypoint_owns_cleanup_scope protects readiness and exercise in the real runner."""
        source = ast.parse(Path(__file__).with_name('legacy-browser.py').read_text())
        scopes = [node for node in ast.walk(source) if isinstance(node, ast.With)
                  and any(isinstance(item.context_expr, ast.Call)
                          and isinstance(item.context_expr.func, ast.Name)
                          and item.context_expr.func.id == 'process_cleanup'
                          for item in node.items)]
        self.assertEqual(len(scopes), 1, 'The real browser runner must use the failure-preserving scope')
        calls = [node for node in ast.walk(scopes[0]) if isinstance(node, ast.Call)]
        self.assertTrue(any(isinstance(node.func, ast.Name) and node.func.id == 'urlopen' for node in calls))
        self.assertTrue(any(isinstance(node.func, ast.Name) and node.func.id == 'exercise' for node in calls))

    def test_real_child_is_reaped_after_interrupted_wait(self) -> None:
        """test_real_child_is_reaped_after_interrupted_wait proves cancellation does not leak a child."""
        script = "import signal,time; signal.signal(signal.SIGTERM, signal.SIG_IGN); print('ready',flush=True); time.sleep(60)"
        with subprocess.Popen([sys.executable, '-u', '-c', script], start_new_session=True, stdout=subprocess.PIPE, text=True) as process:
            wait = process.wait
            original = KeyboardInterrupt('injected wait interruption')
            try:
                with selectors.DefaultSelector() as selector:
                    selector.register(process.stdout, selectors.EVENT_READ)
                    self.assertTrue(selector.select(timeout=5), 'The child must announce readiness')
                self.assertEqual(process.stdout.readline().strip(), 'ready')
                calls = 0

                def interrupted_wait(timeout=None):
                    """interrupted_wait raises once, then delegates bounded reaping to the real child."""
                    nonlocal calls
                    calls += 1
                    if calls == 1:
                        raise original
                    return wait(timeout=timeout)

                with patch.object(process, 'wait', side_effect=interrupted_wait):
                    with self.assertRaises(KeyboardInterrupt) as caught:
                        browser_process.stop_process(process, timeout=1)
                self.assertIs(caught.exception, original)
                self.assertEqual(process.returncode, -signal.SIGKILL, 'Interrupted cleanup must kill and reap the actual child')
            finally:
                # The negative control must not depend on the helper under test.
                with contextlib.suppress(ProcessLookupError):
                    process.kill()
                wait(timeout=5)


if __name__ == '__main__':
    unittest.main(verbosity=2)
