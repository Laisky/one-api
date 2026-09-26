"""Test the actual compiled load driver over loopback HTTP, including intentionally invalid SSE."""
from __future__ import annotations
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from types import SimpleNamespace
import run


class HarnessTests(unittest.TestCase):
    """HarnessTests prove the measurement client rejects false success and respects fixture boundaries."""
    @classmethod
    def setUpClass(cls):
        """setUpClass builds the dependency-free driver once using the available Go toolchain."""
        cls.directory = tempfile.TemporaryDirectory(prefix='stream-driver-test-')
        cls.root = Path(cls.directory.name)
        cls.driver = cls.root / 'driver'
        source = Path(__file__).parent
        subprocess.run(['go', 'build', '-o', str(cls.driver), str(source / 'main.go'), str(source / 'protocol.go')],
                       env={**os.environ, 'GO111MODULE': 'off'}, check=True, timeout=180)

    @classmethod
    def tearDownClass(cls):
        """tearDownClass removes all transient binaries after every child has been reaped."""
        cls.directory.cleanup()

    def setUp(self):
        """setUp starts an authenticated mock server on an unused loopback port."""
        port = run.free_port()
        self.url = f'http://127.0.0.1:{port}'
        self.env = {**os.environ, 'STREAM_PERF_UPSTREAM_TOKEN': 'fixture-only', 'STREAM_PERF_TOKEN': 'fixture-only'}
        self.mock = subprocess.Popen([str(self.driver), '-mode', 'mock', '-listen', f'127.0.0.1:{port}'], env=self.env,
                                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.addCleanup(run.stop, self.mock)
        run.wait_ready(self.url + '/health', self.mock)
        self.args = SimpleNamespace(driver=self.driver, chunk_bytes=128, rate=0)

    def test_delivery_and_false_success(self):
        """test_delivery_and_false_success requires exact content, usage, stop and DONE through HTTP EOF."""
        for fault in ('', 'crlf', 'fragmented', 'missing-done', 'duplicate-done', 'wrong-content', 'no-usage', 'malformed', 'error'):
            with self.subTest(fault=fault):
                cmd = run.driver_command(self.args, self.url + '/v1/chat/completions', self.root / 'result.json',
                                         'contract', 4, 8, 4, 1, fault)
                good = fault in ('', 'crlf', 'fragmented')
                result = run.run_driver(cmd, self.env, good)
                self.assertEqual(result['completed'], 8 if good else 0)
                self.assertEqual(result['failed'], 0 if good else 8)
                self.assertEqual(result['offered'], len(result['samples']) + result['dropped'])
                if good:
                    self.assertGreater(result['ttft_ms']['p50'], .5)
                    self.assertLessEqual(result['ttft_ms']['p50'], result['done_ms']['p50'])

    def test_overload_is_counted(self):
        """test_overload_is_counted offers requests faster than one worker can finish and requires visible drops."""
        self.args.rate = 1000
        cmd = run.driver_command(self.args, self.url + '/v1/chat/completions', self.root / 'overload.json',
                                 'overload', 1, 100, 20, 5)
        result = run.run_driver(cmd, self.env, False)
        self.assertGreater(result['dropped'], 0)
        self.assertEqual(result['offered'], result['completed'] + result['failed'] + result['dropped'])

    def test_external_targets_are_rejected(self):
        """test_external_targets_are_rejected prevents accidental load against non-local services or authenticated URLs."""
        for target in ('https://example.com/v1/chat/completions', 'http://10.0.0.1/', 'http://user:secret@127.0.0.1/'):
            result = subprocess.run([str(self.driver), '-url', target], env=self.env, capture_output=True, timeout=10)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b'loopback', result.stderr)

    def test_linux_resource_accounting(self):
        """test_linux_resource_accounting checks stat field indexing against the current live process."""
        cpu, rss = run.proc_sample(os.getpid())
        self.assertGreaterEqual(cpu, 0)
        self.assertGreater(rss, 0)
        monitor = run.Resources({'self': os.getpid()})
        evidence = monitor.finish(1, 1)
        self.assertGreater(evidence['self']['peak_rss_mib'], 0)


if __name__ == '__main__':
    unittest.main()
