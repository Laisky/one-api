"""Guard the single-variable diagnostic client control independently from gateway and mock settings."""
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

import run


class ClientProcessTests(unittest.TestCase):
    """ClientProcessTests observe actual fixture environments without launching heavy gateway processes."""

    def launch(self, **options):
        """launch captures exact environment dictionaries and requires both synthetic children to be reaped."""
        with tempfile.TemporaryDirectory() as directory:
            args = SimpleNamespace(driver=Path('/fixture-driver'), token_cache=Path(directory), debug_logs=False)
            with mock.patch.object(run.subprocess, 'Popen', side_effect=[SimpleNamespace(pid=101), SimpleNamespace(pid=102)]) as start, \
                 mock.patch.object(run, 'wait_ready'), mock.patch.object(run, 'api', return_value={'success': True}), \
                 mock.patch.object(run, 'stop') as stop:
                with run.fixture(args, Path('/fixture-gateway'), **options) as fixture:
                    output = fixture.copy()
                    environments = [call.kwargs['env'] for call in start.call_args_list]
                self.assertEqual(stop.call_count, 2)
                return environments, output

    def test_only_client_slots_change(self):
        """test_only_client_slots_change leaves gateway/mock configuration, profiling and credentials independent."""
        environments, fixture = self.launch(gateway_procs=3, auxiliary_procs=2, client_procs=4)
        self.assertEqual(environments[0]['GOMAXPROCS'], '2')
        self.assertEqual(environments[1]['GOMAXPROCS'], '3')
        self.assertEqual(fixture['env']['GOMAXPROCS'], '4')
        self.assertEqual(fixture['upstream_env']['GOMAXPROCS'], '4')
        self.assertEqual(fixture['process_slots'], {'gateway': 3, 'mock': 2, 'driver': 4})
        self.assertNotIn('STREAM_PERF_UPSTREAM_TOKEN', fixture['env'])
        self.assertNotIn('INITIAL_ROOT_ACCESS_TOKEN', fixture['env'])
        self.assertNotIn('ENABLE_PPROF', fixture['env'])

    def test_default_ignores_host_environment(self):
        """test_default_ignores_host_environment prevents shell GOMAXPROCS or hidden knobs from changing A/B."""
        with mock.patch.dict(run.os.environ, {'GOMAXPROCS': '63', 'CLIENT_PROCS': '31'}):
            environments, fixture = self.launch()
        self.assertEqual([e['GOMAXPROCS'] for e in environments], ['2', '2'])
        self.assertEqual(fixture['env']['GOMAXPROCS'], '2')
        self.assertEqual(fixture['process_slots'], {'gateway': 2, 'mock': 2, 'driver': 2})

    def test_omitted_client_follows_explicit_auxiliary(self):
        """test_omitted_client_follows_explicit_auxiliary retains the existing public diagnostic default."""
        environments, fixture = self.launch(gateway_procs=3, auxiliary_procs=1)
        self.assertEqual(environments[0]['GOMAXPROCS'], '1')
        self.assertEqual(fixture['env']['GOMAXPROCS'], '1')

    def test_boundaries_and_profile_isolation(self):
        """test_boundaries_and_profile_isolation allows bounded explicit slots without enabling a client listener."""
        for slots in (1, 64):
            with self.subTest(slots=slots):
                environments, fixture = self.launch(gateway_procs=3, auxiliary_procs=2, client_procs=slots, pprof_port=6060)
                self.assertEqual(fixture['env']['GOMAXPROCS'], str(slots))
                self.assertEqual(environments[1]['PPROF_LISTEN'], '127.0.0.1:6060')
                self.assertNotIn('PPROF_LISTEN', fixture['env'])
                self.assertNotIn('ENABLE_PPROF', environments[0])

    def test_invalid_slots_start_no_processes(self):
        """test_invalid_slots_start_no_processes rejects booleans, fractional or unbounded values before provisioning."""
        args = SimpleNamespace()
        for key in ('gateway_procs', 'auxiliary_procs', 'client_procs'):
            for value in (0, -1, 65, True, False, 2.5, '4', float('nan')):
                with self.subTest(key=key, value=value), mock.patch.object(run.subprocess, 'Popen') as start:
                    with self.assertRaisesRegex(ValueError, 'CPU slots'):
                        with run.fixture(args, Path('/unused'), **{key: value}):
                            self.fail('invalid fixture was entered')
                    start.assert_not_called()


if __name__ == '__main__':
    unittest.main()
