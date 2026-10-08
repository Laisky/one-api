"""Integrity checks for the essential Go gate; no application assertions are changed."""

import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("quick_tests", Path(__file__).with_name("quick_tests.py"))
QUICK = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(QUICK)
PACKAGE = QUICK.MODULE + "/relay/format"


def discovery_events(names):
    """discovery_events returns a successful synthetic Go inventory for integrity tests."""
    return [{"Action": "output", "Package": PACKAGE, "Output": "\n".join(names) + "\n"},
            {"Action": "pass", "Package": PACKAGE}]


def execution_events():
    """execution_events returns one successful top-level execution and package result."""
    return [{"Action": "run", "Package": PACKAGE, "Test": "TestEssential"},
            {"Action": "pass", "Package": PACKAGE, "Test": "TestEssential"},
            {"Action": "pass", "Package": PACKAGE}]


class IntegrityTests(unittest.TestCase):
    """IntegrityTests verifies that missing or misleading evidence cannot pass the gate."""

    def test_new_core_tests_are_discovered(self):
        """test_new_core_tests_are_discovered verifies new package tests enter the selection."""
        selected, _ = QUICK.discover(discovery_events(["TestEssential", "TestNew"]),
                                     {"relay/format": None})
        self.assertEqual(selected[PACKAGE], ["TestEssential", "TestNew"])

    def test_required_smoke_absence_fails(self):
        """test_required_smoke_absence_fails rejects renamed or removed essential tests."""
        with self.assertRaises(ValueError):
            QUICK.discover(discovery_events(["TestOther"]),
                           {"relay/format": ("TestEssential",)})

    def test_invalid_inventory_fails(self):
        """test_invalid_inventory_fails rejects empty, duplicate and failed inventories."""
        for events in ([], discovery_events([]), discovery_events(["TestOne", "TestOne"]),
                       discovery_events(["TestOne"]) + [{"Action": "fail"}]):
            with self.subTest(events=events), self.assertRaises(ValueError):
                QUICK.discover(events, {"relay/format": None})

    def test_valid_execution_passes(self):
        """test_valid_execution_passes verifies successful execution counts."""
        self.assertEqual(QUICK.validate_execution(execution_events(),
                                                 {PACKAGE: ["TestEssential"]}),
                         {PACKAGE: 1})

    def test_bad_execution_fails(self):
        """test_bad_execution_fails rejects skips, failures and incomplete or duplicated results."""
        valid = execution_events()
        invalid = [valid[:-1], valid[1:], valid + valid,
                   valid + [{"Action": "skip", "Package": PACKAGE,
                             "Test": "TestEssential/child"}],
                   valid + [{"Action": "build-fail"}],
                   valid + [{"Action": "run", "Package": PACKAGE, "Test": "TestUnexpected"}]]
        for events in invalid:
            with self.subTest(events=events), self.assertRaises(ValueError):
                QUICK.validate_execution(events, {PACKAGE: ["TestEssential"]})

    def test_invalid_json_fails(self):
        """test_invalid_json_fails rejects malformed and empty process evidence."""
        for output in ("", "not json", "[]", '{}'):
            with self.subTest(output=output), self.assertRaises(ValueError):
                QUICK.parse_events(output)

    def test_nonzero_exit_cannot_be_masked_by_valid_json(self):
        """test_nonzero_exit_cannot_be_masked_by_valid_json preserves the real Go exit."""
        result = subprocess.CompletedProcess(["go"], 7, '{"Action":"pass"}\n', "build failed")
        with tempfile.TemporaryDirectory() as directory, patch.object(
                QUICK.subprocess, "run", return_value=result):
            output = Path(directory)
            with self.assertRaises(ValueError):
                QUICK.run_go("go", [], output, "execution", time.monotonic() + 30)
            receipt = json.loads((output / "execution.receipt.json").read_text())
            self.assertEqual(receipt["exit_code"], 7)
            self.assertEqual((output / "execution.stderr.txt").read_text(), "build failed")

    def test_timeout_preserves_partial_evidence(self):
        """test_timeout_preserves_partial_evidence records timeout without inventing an exit."""
        error = subprocess.TimeoutExpired(["go"], 1, output=b'{"Action":"start"}\n')
        with tempfile.TemporaryDirectory() as directory, patch.object(
                QUICK.subprocess, "run", side_effect=error):
            output = Path(directory)
            with self.assertRaises(subprocess.TimeoutExpired):
                QUICK.run_go("go", [], output, "execution", time.monotonic() + 30)
            receipt = json.loads((output / "execution.receipt.json").read_text())
            self.assertIsNone(receipt["exit_code"])
            self.assertEqual(receipt["status"], "timed_out")
            self.assertIn('"start"', (output / "execution.jsonl").read_text())


if __name__ == "__main__":
    unittest.main()
