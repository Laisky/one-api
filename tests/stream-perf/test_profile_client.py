"""Require explicit isolated client capture without silently qualifying a missing trace."""
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest import mock
import profile_client
import run


class ClientProfileTests(unittest.TestCase):
    """ClientProfileTests keep default runs inert, enforce descriptor checks, and reject out-of-window observations."""

    def test_default_is_inert_and_cleans_inherited_activation(self):
        """test_default_is_inert_and_cleans_inherited_activation prevents shell environment from enabling a diagnostic."""
        original = {profile_client.ENVIRONMENT:'0.0.0.0:6060','EXAMPLE':'keep'}
        env, port = profile_client.configuration(SimpleNamespace(mode='trace'), original)
        self.assertEqual(env, {'EXAMPLE':'keep'});self.assertIsNone(port)
        self.assertIn(profile_client.ENVIRONMENT, original)

    def test_explicit_mode_and_loopback_address(self):
        """test_explicit_mode_and_loopback_address creates one private listener only for trace mode."""
        with mock.patch.object(run,'free_port',return_value=12345):
            env, port = profile_client.configuration(SimpleNamespace(mode='trace',client_trace=True), {})
        self.assertEqual(port,12345);self.assertEqual(env[profile_client.ENVIRONMENT],'127.0.0.1:12345')
        for mode in ('cpu','heap','steady'):
            with self.assertRaises(ValueError):
                profile_client.configuration(SimpleNamespace(mode=mode,client_trace=True), {})

    def test_descriptor_validation_precedes_capture(self):
        """test_descriptor_validation_precedes_capture refuses a different process or externally bound listener."""
        with mock.patch.object(profile_client,'assert_loopback_listeners',side_effect=RuntimeError('wrong listener')), \
             mock.patch.object(profile_client,'capture') as capture:
            with self.assertRaises(RuntimeError): profile_client.collect(42,6060,5,Path('/unused'))
            capture.assert_not_called()
        with mock.patch.object(profile_client,'assert_loopback_listeners') as descriptors, \
             mock.patch.object(profile_client,'capture',return_value={'bytes':10}) as capture:
            result=profile_client.collect(42,6060,5,Path('/unused'))
            descriptors.assert_called_once_with(42,{6060})
            self.assertEqual(result['process_role'],'driver')
            self.assertEqual(capture.call_args.args[0],'http://127.0.0.1:6060/debug/pprof/trace?seconds=5')

    def test_missing_and_out_of_window_capture_fail(self):
        """test_missing_and_out_of_window_capture_fail keeps gateway-only success from certifying client evidence."""
        self.assertFalse(profile_client.inside(None,100,30,60))
        for start,end in ((129,150),(145,191),(150,145)):
            self.assertFalse(profile_client.inside({'process_role':'driver','started_monotonic':start,'finished_monotonic':end},100,30,60))
        self.assertTrue(profile_client.inside({'process_role':'driver','started_monotonic':145,'finished_monotonic':150},100,30,60))
