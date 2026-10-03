"""Behavioral tests for pinned tokenizer preparation; no test uses the network."""
from __future__ import annotations
import hashlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import cache_tokens


class TokenCacheTests(unittest.TestCase):
    """TokenCacheTests verifies offline behavior, integrity failures and atomic publication."""

    def setUp(self):
        """setUp installs a small pinned fixture and denies unexpected network requests."""
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.cache = Path(self.directory.name) / 'cache'
        self.payload = b'verified test tokenizer'
        self.assets = mock.patch.dict(cache_tokens.ASSETS, {'fixture': hashlib.sha256(self.payload).hexdigest()}, clear=True)
        self.assets.start()
        self.addCleanup(self.assets.stop)
        url = 'https://openaipublic.blob.core.windows.net/encodings/fixture.tiktoken'
        self.target = self.cache / hashlib.sha1(url.encode()).hexdigest()

    def test_check_only_never_downloads_or_creates_cache(self):
        """test_check_only_never_downloads_or_creates_cache rejects absent assets without side effects."""
        with mock.patch.object(cache_tokens.urllib.request, 'urlopen', side_effect=AssertionError('unexpected network')) as network:
            with self.assertRaisesRegex(ValueError, 'missing or corrupt'):
                cache_tokens.prepare(self.cache, check_only=True)
            network.assert_not_called()
        self.assertFalse(self.cache.exists())

    def test_corrupt_supplied_cache_is_not_repaired_offline(self):
        """test_corrupt_supplied_cache_is_not_repaired_offline preserves the invalid file for diagnosis."""
        self.cache.mkdir()
        self.target.write_bytes(b'corrupt')
        with mock.patch.object(cache_tokens.urllib.request, 'urlopen', side_effect=AssertionError('unexpected network')):
            with self.assertRaisesRegex(ValueError, 'missing or corrupt'):
                cache_tokens.prepare(self.cache, check_only=True)
        self.assertEqual(b'corrupt', self.target.read_bytes())
        self.assertEqual([self.target], list(self.cache.iterdir()))

    def test_verified_cache_works_offline_in_both_modes(self):
        """test_verified_cache_works_offline_in_both_modes accepts a valid cache without downloading."""
        self.cache.mkdir()
        self.target.write_bytes(self.payload)
        with mock.patch.object(cache_tokens.urllib.request, 'urlopen', side_effect=AssertionError('unexpected network')):
            cache_tokens.prepare(self.cache, check_only=True)
            cache_tokens.prepare(self.cache)
        self.assertEqual(self.payload, self.target.read_bytes())

    def test_download_publishes_only_verified_bytes(self):
        """test_download_publishes_only_verified_bytes atomically replaces a corrupt cached file."""
        self.cache.mkdir()
        self.target.write_bytes(b'old corrupt bytes')
        with mock.patch.object(cache_tokens.urllib.request, 'urlopen', return_value=io.BytesIO(self.payload)) as network:
            cache_tokens.prepare(self.cache)
            network.assert_called_once()
        self.assertEqual(self.payload, self.target.read_bytes())
        self.assertEqual([self.target], list(self.cache.iterdir()))

    def test_bad_download_keeps_existing_bytes(self):
        """test_bad_download_keeps_existing_bytes rejects a checksum mismatch without publishing it."""
        self.cache.mkdir()
        self.target.write_bytes(b'old corrupt bytes')
        with mock.patch.object(cache_tokens.urllib.request, 'urlopen', return_value=io.BytesIO(b'wrong response')):
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                cache_tokens.prepare(self.cache)
        self.assertEqual(b'old corrupt bytes', self.target.read_bytes())
        self.assertEqual([self.target], list(self.cache.iterdir()))

    def test_failed_publication_cleans_temporary_file(self):
        """test_failed_publication_cleans_temporary_file leaves no partial asset after a filesystem failure."""
        with mock.patch.object(cache_tokens.urllib.request, 'urlopen', return_value=io.BytesIO(self.payload)):
            with mock.patch.object(Path, 'replace', side_effect=OSError('publication failed')):
                with self.assertRaisesRegex(OSError, 'publication failed'):
                    cache_tokens.prepare(self.cache)
        self.assertEqual([], list(self.cache.iterdir()))


if __name__ == '__main__':
    unittest.main()
