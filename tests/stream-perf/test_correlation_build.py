"""Protect inert default builds, pinned overlay inputs, boundaries and no-overwrite semantics."""
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import correlation_build as build

ROOT = Path(__file__).resolve().parents[2]


class CorrelationBuildTests(unittest.TestCase):
    """CorrelationBuildTests test real pinned inputs without compiling or modifying the gateway."""

    def setUp(self):
        """setUp snapshots exact inputs so every test can prove production files remained unchanged."""
        self.sources = {name: (ROOT / name).read_bytes() for name in build.PINS}

    def test_overlay_preserves_writes_and_bounds(self):
        """test_overlay_preserves_writes_and_bounds inserts clocks but does not replace the render or flush operation."""
        transformed = build.overlay_sources(self.sources)
        render = transformed['common/render/render.go']
        original = self.sources['common/render/render.go'].decode()
        self.assertEqual(render.count('c.Writer.Flush()'), original.count('c.Writer.Flush()'))
        self.assertEqual(render.count('c.Render('), original.count('c.Render('))
        self.assertLess(render.index('observed.BeforeFlush()'), render.index('c.Writer.Flush()'))
        self.assertLess(render.index('c.Writer.Flush()'), render.index('observed.AfterFlush()'))
        self.assertNotIn('observed.', render[render.index('func SSEEvent'):])
        driver = transformed['tests/stream-perf/main.go']
        self.assertIn('o.requests > observation.MaxRequests || o.chunks > 1024', driver)
        self.assertIn('if s.observe { req.Header.Set', driver)
        protocol = transformed['tests/stream-perf/protocol.go']
        self.assertLess(protocol.index('observation.ObserveClient'), protocol.index('if payload == "[DONE]"'))
        self.assertEqual(self.sources, {name: (ROOT / name).read_bytes() for name in build.PINS})

    def test_changed_input_is_rejected_before_output_creation(self):
        """test_changed_input_is_rejected_before_output_creation refuses to instrument a silently changed renderer."""
        altered = dict(self.sources)
        altered['common/render/render.go'] += b'\n'
        with self.assertRaisesRegex(ValueError, 'unsupported source'):
            build.overlay_sources(altered)
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'output'
            with patch.object(build, 'overlay_sources', side_effect=ValueError('drift')):
                with self.assertRaises(ValueError):
                    build.prepare(ROOT, target)
            self.assertFalse(target.exists())

    def test_exclusive_output_and_exact_manifest(self):
        """test_exclusive_output_and_exact_manifest creates only a detached overlay and refuses evidence replacement."""
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'overlay'
            manifest = build.prepare(ROOT, target)
            self.assertTrue(manifest['diagnostic_only'])
            self.assertEqual(set(manifest['sources']), set(build.PINS))
            for name in build.PINS:
                self.assertEqual((target / name).read_text(), build.overlay_sources(self.sources)[name])
            with self.assertRaises(FileExistsError):
                build.prepare(ROOT, target)
        self.assertEqual(self.sources, {name: (ROOT / name).read_bytes() for name in build.PINS})

    def test_anchor_must_be_unique(self):
        """test_anchor_must_be_unique never replaces the first of several similar production statements."""
        for text in ('missing', 'xx'):
            with self.assertRaises(ValueError):
                build.replace_once(text, 'x', 'injected')
