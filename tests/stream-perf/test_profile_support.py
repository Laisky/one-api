"""Behavior guards for gateway-only sustained-load admission and explicit profiler activation."""
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

import profile_support
import run


def samples(gateway_cores=2.8, auxiliary_cores=0.5, seconds=92):
    """samples builds monotonic one-second records whose gateway/auxiliary CPU attribution is independent."""
    return [{'elapsed': float(i), 'processes': {'gateway': {'cpu_seconds': i * gateway_cores},
                                              'driver': {'cpu_seconds': i * auxiliary_cores}},
             'upstream': {'completed': i * 50, 'active': 8}, 'durable_requests': i * 50} for i in range(seconds)]


class ProfileAdmissionTests(unittest.TestCase):
    """ProfileAdmissionTests reject idle gateways, incomplete windows and corrupted samples without hiding failure."""

    def test_busy_generator_cannot_satisfy_gateway_gate(self):
        """test_busy_generator_cannot_satisfy_gateway_gate keeps a busy auxiliary process out of the gateway numerator."""
        result = profile_support.stable_window(samples(.5, 3.5), 4, 4, 30, 60)
        self.assertFalse(result['qualified'])
        self.assertEqual(result['fraction_at_or_above_target'], 0)

    def test_effective_allowance_and_slots_are_distinct(self):
        """test_effective_allowance_and_slots_are_distinct does not label two saturated slots as a saturated four-core machine."""
        result = profile_support.stable_window(samples(1.8), 4, 2, 30, 60)
        self.assertFalse(result['qualified'])
        self.assertAlmostEqual(result['intervals'][0]['machine_utilization'], .45)
        self.assertAlmostEqual(result['intervals'][0]['gateway_slot_utilization'], .9)

    def test_sustained_gateway_load_is_admitted(self):
        """test_sustained_gateway_load_is_admitted requires the declared complete, individually qualifying window."""
        result = profile_support.stable_window(samples(), 4, 4, 30, 60)
        self.assertTrue(result['qualified'])
        self.assertEqual(result['coverage_seconds'], 60)
        self.assertEqual(len(result['intervals']), 60)
        self.assertAlmostEqual(result['gateway_cores_mean'], 2.8)
        self.assertEqual(result['upstream_active_peak'], 8)

    def test_truncated_and_gapped_windows_fail(self):
        """test_truncated_and_gapped_windows_fail rejects incomplete coverage even when all remaining samples are busy."""
        self.assertFalse(profile_support.stable_window(samples(seconds=70), 4, 4, 30, 60)['qualified'])
        rows = samples()
        del rows[45:50]
        self.assertFalse(profile_support.stable_window(rows, 4, 4, 30, 60)['qualified'])

    def test_cpu_counter_rollback_is_invalid(self):
        """test_cpu_counter_rollback_is_invalid rejects mixed or corrupt process counters rather than clipping them."""
        rows = samples()
        rows[40]['processes']['gateway']['cpu_seconds'] = 0
        with self.assertRaisesRegex(ValueError, 'non-monotonic'):
            profile_support.stable_window(rows, 4, 4, 30, 60)

    def test_invalid_allowance_is_rejected(self):
        """test_invalid_allowance_is_rejected prevents undefined or misleading utilization percentages."""
        for cores in (0, -1, float('nan'), float('inf')):
            with self.subTest(cores=cores), self.assertRaises(ValueError):
                profile_support.stable_window(samples(), cores, 4, 30, 60)

    def test_failed_window_remains_serializable(self):
        """test_failed_window_remains_serializable preserves negative conclusions and drift in an atomic checkpoint."""
        result = profile_support.stable_window(samples(.3), 4, 4, 30, 60)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'summary.json'
            profile_support.write_json(path, result)
            self.assertIn('"qualified": false', path.read_text())
            self.assertFalse(path.with_suffix('.json.tmp').exists())


class ProfileFixtureTests(unittest.TestCase):
    """ProfileFixtureTests observe actual launcher arguments without running an expensive gateway."""

    def launch(self, **options):
        """launch records environments for a real fixture context with stand-in child processes and management requests."""
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

    def test_default_fixture_does_not_inherit_profiling(self):
        """test_default_fixture_does_not_inherit_profiling keeps normal A/B runs unprofiled even in a profiling shell."""
        with mock.patch.dict(run.os.environ, {'ENABLE_PPROF': 'true', 'PPROF_LISTEN': '0.0.0.0:6060', 'GOMAXPROCS': '63'}):
            environments, fixture = self.launch()
        gateway = environments[1]
        self.assertEqual(gateway['LISTEN_HOST'], '127.0.0.1')
        self.assertEqual(gateway['GOMAXPROCS'], '2')
        self.assertNotIn('ENABLE_PPROF', gateway)
        self.assertNotIn('PPROF_LISTEN', gateway)
        self.assertIsNone(fixture['pprof_url'])

    def test_diagnostic_controls_are_explicit_and_loopback_only(self):
        """test_diagnostic_controls_are_explicit_and_loopback_only changes only explicitly requested process slots and pprof."""
        environments, fixture = self.launch(gateway_procs=4, auxiliary_procs=1, pprof_port=6060)
        self.assertEqual(environments[0]['GOMAXPROCS'], '1')
        self.assertEqual(environments[1]['GOMAXPROCS'], '4')
        self.assertEqual(environments[1]['PPROF_LISTEN'], '127.0.0.1:6060')
        self.assertEqual(environments[1]['ENABLE_PPROF'], 'true')
        self.assertEqual(fixture['env']['GOMAXPROCS'], '1')
        self.assertEqual(fixture['pprof_url'], 'http://127.0.0.1:6060/debug/pprof')

    def test_invalid_process_configuration_starts_nothing(self):
        """test_invalid_process_configuration_starts_nothing bounds diagnostic knobs before any child is launched."""
        for options in ({'gateway_procs': 0}, {'auxiliary_procs': 65}, {'pprof_port': 65536}):
            with self.subTest(options=options), self.assertRaises(ValueError):
                self.launch(**options)
