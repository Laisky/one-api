"""Exercise real formatter acceptance and rejection without altering repository source."""
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from format_check import check_files


class FormatCheckTests(unittest.TestCase):
    """FormatCheckTests protect fail-closed formatting across Linux and Windows checkouts."""

    def test_empty_and_duplicate_inventories_fail(self) -> None:
        """test_empty_and_duplicate_inventories_fail rejects falsely successful discovery."""
        for files in ([], ["a.go", "a.go"]):
            with self.assertRaises(ValueError):
                check_files(Path("."), files)

    def test_real_formatter_preserves_crlf_source_and_rejects_drift(self) -> None:
        """test_real_formatter_preserves_crlf_source_and_rejects_drift checks actual formatter output."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "a.go"
            original = b"package example\r\n\r\nfunc Valid() {}\r\n"
            source.write_bytes(original)
            check_files(root, ["a.go"])
            self.assertEqual(source.read_bytes(), original)
            source.write_bytes(b"package example\nfunc Invalid( ){}\n")
            with self.assertRaisesRegex(ValueError, "require gofmt"):
                check_files(root, ["a.go"])

    def test_formatter_error_propagates(self) -> None:
        """test_formatter_error_propagates prevents an unavailable formatter from passing."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "a.go").write_text("package example\n")
            with patch("format_check.subprocess.run", side_effect=subprocess.CalledProcessError(2, "gofmt")):
                with self.assertRaises(subprocess.CalledProcessError):
                    check_files(root, ["a.go"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
