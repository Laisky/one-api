"""Guard media closure alias identity, origin paths and notice preservation."""
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import collect_media_runtime as collect


class MediaCollectionTests(unittest.TestCase):
    """MediaCollectionTests reject unsafe relocation and missing notice artifacts."""

    def test_origin_paths_remain_inside_private_runtime(self):
        """test_origin_paths permits relative lookups only inside the selected tree."""
        with tempfile.TemporaryDirectory() as parent:
            media = Path(parent) / "opt/media"
            destination = media / "lib/library.so"
            with patch.object(collect, "MEDIA", media), \
                    patch.object(collect, "ROOT", Path(parent)), \
                    patch.object(collect, "ELF_FILES", {}):
                collect.validate_lookup({"lookup_paths": ["$ORIGIN:$ORIGIN/../lib"]}, destination)
                for paths in ("$ORIGIN/../../../outside", "/usr/local/lib", "$ORIGIN:",
                              "$ORIGIN/$PLATFORM"):
                    with self.subTest(paths=paths), self.assertRaises(ValueError):
                        collect.validate_lookup({"lookup_paths": [paths]}, destination)

    def test_conflicting_soname_alias_is_rejected(self):
        """test_conflicting_soname prevents a different file from replacing an alias."""
        with tempfile.TemporaryDirectory() as parent:
            root = Path(parent)
            first, second = root / "first.so", root / "second.so"
            first.write_bytes(b"one version")
            second.write_bytes(b"different version")
            destination = root / "runtime/libsame.so.1"
            with patch.object(collect, "ROOT", root), patch.object(collect, "COPIED", {}), \
                    patch.object(collect, "RECORDS", []):
                collect.copy_file(first, destination, role="library")
                with self.assertRaisesRegex(ValueError, "different libraries share alias"):
                    collect.copy_file(second, destination, role="library")
                self.assertEqual(destination.read_bytes(), first.read_bytes())

    def test_valid_notice_links_are_copied_and_broken_news_is_recorded(self):
        """test_valid_notice_links retains real notice bytes without a broken doc link."""
        with tempfile.TemporaryDirectory() as parent:
            root = Path(parent)
            source = root / "source"
            source.mkdir()
            (source / "notice.txt").write_text("license content")
            (source / "copyright").symlink_to("notice.txt")
            (source / "NEWS").symlink_to("absent.gz")
            with patch.object(collect, "DANGLING_DOC_LINKS", []):
                collect.copy_tree(source, root / "output", documentation=True)
                self.assertEqual((root / "output/copyright").read_text(), "license content")
                self.assertFalse((root / "output/NEWS").exists())
                self.assertEqual(len(collect.DANGLING_DOC_LINKS), 1)

    def test_broken_notice_link_is_rejected(self):
        """test_broken_notice fails instead of silently omitting a required notice."""
        with tempfile.TemporaryDirectory() as parent:
            root = Path(parent)
            source = root / "source"
            source.mkdir()
            (source / "copyright").symlink_to("absent.txt")
            with self.assertRaisesRegex(ValueError, "source notice target is missing"):
                collect.copy_tree(source, root / "output", documentation=True)


if __name__ == "__main__":
    unittest.main(verbosity=2)
