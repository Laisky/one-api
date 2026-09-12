#!/usr/bin/env python3
"""Test discovery, real Go JSON and coverage merging without network dependencies.

Run after setup-go on the packages shard. These synthetic fixtures test the
orchestrator, not application behavior or live-database qualification.
"""
from __future__ import annotations

import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import go_test_shards as runner


class GoToolIntegrationTests(unittest.TestCase):
    """GoToolIntegrationTests exercise orchestration with real Go subprocesses."""

    def test_actual_go_inventory_fuzz_examples_coverage_and_failure(self) -> None:
        """test_actual_go_inventory_fuzz_examples_coverage_and_failure checks the full pipeline."""
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "repo"
            root.mkdir()
            (root / "go.mod").write_text("module example.test/shards\n\ngo 1.23\n")
            files = {
                "model/value.go": "package model\nfunc Value() int { return 7 }\n",
                "model/value_test.go": '''package model
import ("fmt"; "testing")
func TestAlpha(t *testing.T) { if Value() != 7 { t.Fatal("value") } }
func TestAlphabet(t *testing.T) {
    t.Run("child/nested", func(t *testing.T) { if Value() != 7 { t.Fatal("value") } })
}
func TestGamma(t *testing.T) { t.Skip("intentional optional fixture") }
func TestDelta(t *testing.T) { if Value() != 7 { t.Fatal("value") } }
func FuzzValue(f *testing.F) {
    f.Add(7)
    f.Fuzz(func(t *testing.T, n int) { if Value() != 7 { t.Fatal(n) } })
}
func ExampleValue() {
    fmt.Println(Value())
    // Output: 7
}
''',
                "other/value.go": "package other\nfunc Value() int { return 9 }\n",
                "other/value_test.go": '''package other
import "testing"
func TestOther(t *testing.T) { if Value() != 9 { t.Fatal("value") } }
''',
                "untested/value.go": "package untested\nfunc Value() int { return 11 }\n",
            }
            for name, content in files.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(content)
            evidence = Path(temp) / "evidence"
            env = {"GITHUB_SHA": "fixture-revision", "GITHUB_STEP_SUMMARY": "",
                   "GOWORK": "off", "GOPROXY": "off", "GOSUMDB": "off"}
            with mock.patch.object(runner, "ROOT", root), mock.patch.dict(os.environ, env):
                for shard in runner.SHARDS:
                    self.assertEqual(runner.run_shard(shard, evidence / shard), 0, shard)
                merged = Path(temp) / "coverage.txt"
                runner.merge_shards(evidence, merged)
                expected = {"TestAlpha", "TestAlphabet", "TestGamma", "TestDelta", "FuzzValue", "ExampleValue"}
                selected, events = [], []
                for shard in runner.SHARDS[1:]:
                    manifest = json.loads((evidence / shard / "manifest.json").read_text())
                    selected.extend(manifest["selected_tests"])
                    events.extend(runner.read_events(evidence / shard / "events.jsonl"))
                self.assertCountEqual(selected, expected)
                for child in ("TestAlphabet/child/nested", "FuzzValue/seed#0"):
                    self.assertTrue(any(e.get("Test") == child and e["Action"] == "pass" for e in events))
                self.assertTrue(any(e.get("Test") == "TestGamma" and e["Action"] == "skip" for e in events))
                self.assertEqual(merged.read_text().count("mode: atomic"), 1)
                self.assertEqual(sum("model/value.go:" in line for line in merged.read_text().splitlines()), 1)
                (root / "other/value_test.go").write_text('''package other
import "testing"
func TestOther(t *testing.T) { t.Fatal("intentional negative control") }
''')
                failed = Path(temp) / "failed"
                self.assertNotEqual(runner.run_shard("packages", failed), 0)
                self.assertNotEqual(json.loads((failed / "manifest.json").read_text())["exit_code"], 0)


if __name__ == "__main__":
    unittest.main(verbosity=2)
