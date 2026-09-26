"""Loopback capture tests protect diagnostic identity, bounds and failure evidence."""
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
from pathlib import Path
import tempfile
import threading
import unittest
from unittest import mock
import urllib.error

import profile_capture


class CaptureServer(BaseHTTPRequestHandler):
    """CaptureServer supplies local profiles, an empty body and an external redirect for negative controls."""

    def do_GET(self):
        """do_GET responds with fixed synthetic profile data without reading any user credential."""
        if self.path == '/redirect':
            self.send_response(302)
            self.send_header('Location', 'http://192.0.2.1/forbidden')
            self.end_headers()
            return
        self.send_response(200)
        self.end_headers()
        if self.path != '/empty':
            self.wfile.write(b'synthetic trace bytes\x00\xff')

    def log_message(self, *args):
        """log_message suppresses test-server console noise without dropping application errors."""


class ProfileCaptureTests(unittest.TestCase):
    """ProfileCaptureTests require valid size/time-bounded evidence rather than silent partial success."""

    @classmethod
    def setUpClass(cls):
        """setUpClass starts one local fixture without external hosts or proxy access."""
        cls.server = ThreadingHTTPServer(('127.0.0.1', 0), CaptureServer)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.url = f'http://127.0.0.1:{cls.server.server_port}'

    @classmethod
    def tearDownClass(cls):
        """tearDownClass stops and joins only the fixture created by this test class."""
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=3)

    def setUp(self):
        """setUp gives each test an isolated empty evidence directory."""
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.path = Path(temporary.name) / 'runtime.trace'

    def test_profile_identity_and_complete_publication(self):
        """test_profile_identity_and_complete_publication verifies exact bytes and measured timing metadata."""
        with mock.patch.dict(profile_capture.os.environ, {'HTTP_PROXY': 'http://192.0.2.1:1', 'NO_PROXY': ''}):
            record = profile_capture.capture(self.url + '/trace', self.path, 3)
        expected = b'synthetic trace bytes\x00\xff'
        self.assertEqual(self.path.read_bytes(), expected)
        self.assertEqual(record['bytes'], len(expected))
        self.assertEqual(record['sha256'], hashlib.sha256(expected).hexdigest())
        self.assertGreaterEqual(record['finished_monotonic'], record['started_monotonic'])
        self.assertEqual(record['wall_seconds'], record['finished_monotonic'] - record['started_monotonic'])
        self.assertFalse(self.path.with_suffix('.trace.partial').exists())

    def test_size_limit_retains_only_incomplete_evidence(self):
        """test_size_limit_retains_only_incomplete_evidence refuses an oversized response before publication."""
        with self.assertRaisesRegex(ValueError, 'byte limit'):
            profile_capture.capture(self.url + '/trace', self.path, 3, maximum_bytes=8)
        self.assertFalse(self.path.exists())
        self.assertTrue(self.path.with_suffix('.trace.partial').exists())
        self.assertLessEqual(self.path.with_suffix('.trace.partial').stat().st_size, 8)

    def test_empty_body_is_not_a_profile(self):
        """test_empty_body_is_not_a_profile prevents HTTP success from becoming false diagnostic success."""
        with self.assertRaisesRegex(ValueError, 'empty'):
            profile_capture.capture(self.url + '/empty', self.path, 3)
        self.assertFalse(self.path.exists())

    def test_redirect_is_not_followed(self):
        """test_redirect_is_not_followed rejects changes to the already verified fixture endpoint."""
        with self.assertRaises(urllib.error.HTTPError) as caught:
            profile_capture.capture(self.url + '/redirect', self.path, 3)
        self.assertEqual(caught.exception.code, 302)
        self.assertFalse(self.path.exists())

    def test_existing_and_partial_files_are_not_overwritten(self):
        """test_existing_and_partial_files_are_not_overwritten keeps earlier attempts intact."""
        for path in (self.path, self.path.with_suffix('.trace.partial')):
            path.write_bytes(b'earlier attempt')
            with self.assertRaises(FileExistsError):
                profile_capture.capture(self.url + '/trace', self.path, 3)
            self.assertEqual(path.read_bytes(), b'earlier attempt')
            path.unlink()

    def test_unsafe_target_and_invalid_bounds_do_not_connect(self):
        """test_unsafe_target_and_invalid_bounds_do_not_connect rejects secrets, remote names and invalid sizes first."""
        for url in ('http://example.com/', 'http://user:secret@127.0.0.1/', 'http://10.0.0.1/', 'https://127.0.0.1/'):
            with self.subTest(url=url), mock.patch.object(profile_capture.urllib.request, 'build_opener') as opener:
                with self.assertRaises(ValueError):
                    profile_capture.capture(url, self.path, 3)
                opener.assert_not_called()
        for bound in (0, -1, True, profile_capture.MAX_CAPTURE_BYTES + 1):
            with self.subTest(bound=bound), self.assertRaises(ValueError):
                profile_capture.capture(self.url + '/trace', self.path, 3, bound)

    def test_wall_deadline_does_not_publish_partial_profile(self):
        """test_wall_deadline_does_not_publish_partial_profile uses a controlled clock rather than sleeps."""
        response = io.BytesIO(b'trace')
        opener = mock.Mock()
        opener.open.return_value = response
        with mock.patch.object(profile_capture.urllib.request, 'build_opener', return_value=opener), \
             mock.patch.object(profile_capture.time, 'monotonic', side_effect=[0, .1, 2]):
            with self.assertRaises(TimeoutError):
                profile_capture.capture(self.url + '/trace', self.path, 1)
        self.assertFalse(self.path.exists())
        self.assertEqual(self.path.with_suffix('.trace.partial').read_bytes(), b'trace')

    def test_final_publication_cannot_clobber_concurrent_result(self):
        """test_final_publication_cannot_clobber_concurrent_result exercises the exclusive hard-link publication."""
        original = profile_capture.os.link

        def competing(source, destination):
            """competing creates a competing result immediately before the atomic publication call."""
            destination.write_bytes(b'concurrent result')
            return original(source, destination)

        with mock.patch.object(profile_capture.os, 'link', side_effect=competing), self.assertRaises(FileExistsError):
            profile_capture.capture(self.url + '/trace', self.path, 3)
        self.assertEqual(self.path.read_bytes(), b'concurrent result')
        self.assertTrue(self.path.with_suffix('.trace.partial').exists())
