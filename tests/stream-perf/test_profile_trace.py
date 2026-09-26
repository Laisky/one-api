"""Deterministic orchestration checks for a trace subwindow inside sustained load."""
from contextlib import ExitStack, contextmanager
import hashlib
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

import profile_run


class Clock:
    """Clock advances sampling time only when the driver loop sleeps, without real delays."""

    def __init__(self):
        """__init__ starts the synthetic monotonic clock at zero."""
        self.at = 0.0

    def monotonic(self):
        """monotonic returns the controlled current instant."""
        return self.at

    def sleep(self, seconds):
        """sleep advances synthetic time while rejecting a broken loop's negative delay."""
        if seconds < 0:
            raise AssertionError('negative sleep')
        self.at += max(seconds, .000001)


class Deferred:
    """Deferred captures a synchronous test operation while exposing a future-like result interface."""

    def __init__(self, function, args):
        """__init__ invokes the fake capture and preserves either its result or error."""
        try:
            self.value, self.error = function(*args), None
        except Exception as error:
            self.value, self.error = None, error

    def result(self, timeout=None):
        """result returns captured evidence or propagates the original capture failure."""
        if self.error is not None:
            raise self.error
        return self.value


class Executor:
    """Executor avoids nondeterministic real-thread execution in the orchestration unit test."""

    def __init__(self, **kwargs):
        """__init__ accepts the standard executor's bounded-worker configuration."""

    def __enter__(self):
        """__enter__ returns this synthetic executor."""
        return self

    def __exit__(self, *args):
        """__exit__ never suppresses orchestration exceptions."""
        return False

    def submit(self, function, *args):
        """submit runs a fake capture now and returns an independently inspectable result."""
        return Deferred(function, args)


class ProfileTraceTests(unittest.TestCase):
    """ProfileTraceTests ensure a five-second trace is never claimed as a full-minute profile."""

    def exercise(self, *, mode='trace', overrun=False, fail=False, client_procs=None,
                 paired=False, client_fail=False, client_overrun=False):
        """exercise drives the actual diagnostic function with synthetic local services and a controlled clock."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            clock, calls = Clock(), []
            args = SimpleNamespace(output=root/'evidence', binary=root/'gateway', driver=root/'driver',
                token_cache=root/'cache', mode=mode, gateway_procs=3, auxiliary_procs=2, client_procs=client_procs,
                concurrency=32, requests=100, chunks=1024, chunk_bytes=128, pace_ms=0,
                warmup=30, seconds=60, deadline=120, trace_seconds=5, trace_offset=15, rate=0, client_trace=paired)
            client = SimpleNamespace(pid=99, poll=lambda: 0 if clock.at >= 100 else None, wait=lambda **kw: 0)

            @contextmanager
            def fixture(*unused, **options):
                """fixture supplies an isolated service contract without starting real subprocesses."""
                self.assertEqual(options['pprof_port'], 6060)
                self.assertEqual(options['client_procs'], client_procs)
                yield {'url': 'http://127.0.0.1:3000/v1/chat/completions', 'pids': {'gateway': 97, 'mock': 98},
                       'root': root, 'env': {'GOMAXPROCS': str(client_procs or 2)},
                       'process_slots': {'gateway': 3, 'mock': 2, 'driver': client_procs or 2}, 'mock_url': 'http://127.0.0.1:3001',
                       'pprof_url': 'http://127.0.0.1:6060/debug/pprof'}

            def qualify(*unused, **options):
                """qualify requires functional checks to use the same requested process settings."""
                self.assertEqual(options, {'gateway_procs': 3, 'auxiliary_procs': 2, 'client_procs': client_procs})
                return {'qualified': True}

            def start(*unused, **options):
                """start supplies a completed synthetic report, independent from trace capture success."""
                (args.output/'requests.json').write_text(json.dumps({'completed': 100, 'failed': 0, 'dropped': 0, 'samples': []}))
                self.assertEqual(options['env']['GOMAXPROCS'], str(client_procs or 2))
                return client

            def snapshot(*unused):
                """snapshot emits intact counters at the current controlled sample time."""
                return {'elapsed': clock.at, 'processes': {'gateway': {'cpu_seconds': clock.at * 2.8}},
                        'upstream': {'completed': int(clock.at * 50), 'active': 32},
                        'durable_requests': int(clock.at * 50)}

            def capture(url, path, timeout):
                """capture models a bounded trace operation without overlapping another diagnostic kind."""
                calls.append((url, clock.at))
                if fail:
                    raise RuntimeError('synthetic capture failure')
                path.write_bytes(b'synthetic trace')
                length = 70 if overrun else (5 if mode == 'trace' else 60)
                return {'file': path.name, 'bytes': path.stat().st_size,
                        'sha256': hashlib.sha256(path.read_bytes()).hexdigest(),
                        'started_monotonic': clock.at, 'finished_monotonic': clock.at + length,
                        'wall_seconds': length}

            def client_capture(pid, port, seconds, path):
                """client_capture independently models the second process and its failure interval."""
                if client_fail:
                    raise RuntimeError('synthetic client capture failure')
                result = capture(f'http://127.0.0.1:{port}/debug/pprof/trace?seconds={seconds}', path, seconds+15)
                result['process_role'] = 'driver'
                if client_overrun:
                    result['finished_monotonic'] = clock.at + 70
                return result

            with ExitStack() as stack:
                for target, name, replacement in ((profile_run.platform, 'platform', lambda: 'synthetic Linux'),
                    (profile_run, 'cpu_allowance', lambda: {'effective_cores': 4, 'stat_path': '/unused'}),
                    (profile_run, 'assert_loopback_listeners', lambda *a: None), (profile_run, 'snapshot', snapshot),
                    (profile_run.profile_client, 'collect', client_capture), (profile_run, 'ThreadPoolExecutor', Executor), (profile_run, 'capture', capture),
                    (profile_run.run, 'qualify', qualify),
                    (profile_run.run, 'free_port', lambda: 6060), (profile_run.run, 'sha256', lambda *a: 'fixture'),
                    (profile_run.run, 'fixture', fixture), (profile_run.run, 'run_driver', lambda *a: {}),
                    (profile_run.subprocess, 'Popen', start), (profile_run.time, 'monotonic', clock.monotonic),
                    (profile_run.time, 'sleep', clock.sleep)):
                    stack.enter_context(mock.patch.object(target, name, replacement))
                stack.enter_context(mock.patch.object(profile_run.run, 'wait_usage', side_effect=[(0, 16), (100, 116)]))
                stop = stack.enter_context(mock.patch.object(profile_run.run, 'stop'))
                if fail or client_fail:
                    with self.assertRaisesRegex(RuntimeError, 'synthetic'):
                        profile_run.diagnose(args)
                    result = json.loads((args.output/'summary.json').read_text())
                else:
                    result = profile_run.diagnose(args)
                stop.assert_called_once_with(client)
            return result, calls

    def test_trace_is_a_separate_five_second_subwindow(self):
        """test_trace_is_a_separate_five_second_subwindow records interval and does not collect CPU/heap too."""
        result, calls = self.exercise()
        self.assertTrue(result['complete'])
        self.assertTrue(result['sustained_protocol_qualified'])
        self.assertTrue(result['trace_window_within_observation'])
        self.assertEqual(calls, [('http://127.0.0.1:6060/debug/pprof/trace?seconds=5', 45.0)])
        self.assertEqual(result['profile_captures'][0]['started_elapsed'], 45)
        self.assertEqual(result['profile_captures'][0]['finished_elapsed'], 50)
        self.assertEqual(result['window']['coverage_seconds'], 60)

    def test_capture_outside_window_cannot_qualify(self):
        """test_capture_outside_window_cannot_qualify distinguishes successful data transfer from valid observation."""
        result, _ = self.exercise(overrun=True)
        self.assertTrue(result['complete'])
        self.assertTrue(result['window']['qualified'])
        self.assertFalse(result['trace_window_within_observation'])
        self.assertFalse(result['sustained_protocol_qualified'])

    def test_cpu_remains_a_full_separate_window(self):
        """test_cpu_remains_a_full_separate_window preserves the preexisting CPU-mode contract."""
        result, calls = self.exercise(mode='cpu')
        self.assertTrue(result['sustained_protocol_qualified'])
        self.assertEqual(calls, [('http://127.0.0.1:6060/debug/pprof/profile?seconds=60', 30.0)])
        self.assertNotIn('trace_window_within_observation', result)

    def test_capture_failure_keeps_incomplete_state_and_cleans_up(self):
        """test_capture_failure_keeps_incomplete_state_and_cleans_up rejects false completed-profile status."""
        result, _ = self.exercise(fail=True)
        self.assertFalse(result['complete'])
        self.assertEqual(result['failure_type'], 'RuntimeError')
        self.assertNotIn('sustained_protocol_qualified', result)

    def test_client_control_is_explicit_in_qualification_capture_and_summary(self):
        """test_client_control_is_explicit_in_qualification_capture_and_summary prevents a control-arm mislabel."""
        result, _ = self.exercise(client_procs=4)
        self.assertTrue(result['sustained_protocol_qualified'])
        self.assertEqual(result['configuration']['client_procs'], 4)
        self.assertEqual(result['process_slots'], {'gateway': 3, 'mock': 2, 'driver': 4})
        self.assertEqual(result['window_started_elapsed'], 30.0)

    def test_default_client_control_retains_auxiliary_slots(self):
        """test_default_client_control_retains_auxiliary_slots preserves historical default fixture behavior."""
        result, _ = self.exercise()
        self.assertIsNone(result['configuration']['client_procs'])
        self.assertEqual(result['process_slots'], {'gateway': 3, 'mock': 2, 'driver': 2})

    def test_paired_trace_uses_two_independent_captures(self):
        """test_paired_trace_uses_two_independent_captures records distinct files without merging process roles."""
        result, calls = self.exercise(paired=True)
        self.assertTrue(result['sustained_protocol_qualified'])
        self.assertEqual(len(calls), 2)
        self.assertEqual(result['profile_captures'][0]['file'], 'runtime.trace')
        self.assertEqual(result['client_trace_capture']['file'], 'client.trace')
        self.assertEqual(result['client_trace_capture']['process_role'], 'driver')
        self.assertTrue(result['client_trace_window_valid'])

    def test_client_trace_failure_is_not_hidden_by_gateway_success(self):
        """test_client_trace_failure_is_not_hidden_by_gateway_success retains an incomplete study."""
        result, _ = self.exercise(paired=True, client_fail=True)
        self.assertFalse(result['complete'])
        self.assertEqual(result['failure_type'], 'RuntimeError')

    def test_client_trace_overrun_rejects_the_study(self):
        """test_client_trace_overrun_rejects_the_study does not weaken the gateway window rule."""
        result, _ = self.exercise(paired=True, client_overrun=True)
        self.assertTrue(result['trace_window_within_observation'])
        self.assertFalse(result['client_trace_window_valid'])
        self.assertFalse(result['sustained_protocol_qualified'])

    def test_paired_trace_preserves_explicit_client_cpu_control(self):
        """test_paired_trace_preserves_explicit_client_cpu_control composes independent CPU-slot and trace controls."""
        for slots in (1, 4):
            with self.subTest(slots=slots):
                result, calls = self.exercise(client_procs=slots, paired=True)
                self.assertTrue(result['sustained_protocol_qualified'])
                self.assertEqual(result['process_slots'], {'gateway': 3, 'mock': 2, 'driver': slots})
                self.assertEqual(result['configuration']['client_procs'], slots)
                self.assertEqual(result['client_trace_capture']['process_role'], 'driver')
                self.assertEqual(len(calls), 2)
