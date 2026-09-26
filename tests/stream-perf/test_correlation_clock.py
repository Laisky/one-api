"""Reject misleading cross-process comparisons when clocks do not share a verified epoch."""
from pathlib import Path
import tempfile
import unittest

from profile_support import clock_domain


class ClockDomainTests(unittest.TestCase):
    """ClockDomainTests exercise actual proc-like symlinks rather than assuming child clocks match."""

    def setUp(self):
        """setUp makes a minimal clock-identity filesystem whose namespaces can diverge independently."""
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.boot = self.root / 'sys/kernel/random/boot_id'
        self.boot.parent.mkdir(parents=True)
        self.boot.write_text('12345678-1234-5678-90ab-1234567890ab\n')
        for name in ('self', '101', '102'):
            path = self.root / name / 'ns/time'
            path.parent.mkdir(parents=True)
            path.symlink_to('time:[42]')

    def test_identical_epochs_are_verified(self):
        """test_identical_epochs_are_verified retains both boot and per-process time-namespace identity."""
        result = clock_domain({'gateway': 101, 'driver': 102}, self.root)
        self.assertEqual(result['clock'], 'CLOCK_MONOTONIC')
        self.assertEqual(set(result['time_namespaces'].values()), {'time:[42]'})
        self.assertEqual(len(result['time_namespaces']), 3)

    def test_different_or_missing_namespace_fails(self):
        """test_different_or_missing_namespace_fails never substitutes wall time or an assumed common clock."""
        path = self.root / '102/ns/time'
        path.unlink()
        with self.assertRaisesRegex(ValueError, 'partial'):
            clock_domain({'gateway': 101, 'driver': 102}, self.root)
        path.symlink_to('time:[43]')
        with self.assertRaisesRegex(ValueError, 'different namespaces'):
            clock_domain({'gateway': 101, 'driver': 102}, self.root)

    def test_invalid_identities_fail(self):
        """test_invalid_identities_fail rejects malformed clocks and booleans masquerading as PIDs."""
        for pid in (0, -1, True, '101'):
            with self.assertRaises(ValueError):
                clock_domain({'gateway': pid}, self.root)
        self.boot.write_text('unknown')
        with self.assertRaises(ValueError):
            clock_domain({'gateway': 101}, self.root)

    def test_all_missing_allows_intervals_but_never_one_way_latency(self):
        """test_all_missing_allows_intervals_but_never_one_way_latency marks absent namespace evidence explicitly."""
        for name in ('self', '101', '102'):
            (self.root / name / 'ns/time').unlink()
        result = clock_domain({'gateway': 101, 'driver': 102}, self.root)
        self.assertFalse(result['absolute_cross_process_verified'])
        self.assertTrue(all(value is None for value in result['time_namespaces'].values()))
        (self.root / '102/ns').rmdir()
        with self.assertRaises(FileNotFoundError):
            clock_domain({'gateway': 101, 'driver': 102}, self.root)
