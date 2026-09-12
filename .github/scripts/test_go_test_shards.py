#!/usr/bin/env python3
"""Regression tests for sharding completeness, Go exit propagation and atomic coverage."""
from __future__ import annotations

import json
import os
from pathlib import Path
import re
import tempfile
import unittest
from unittest.mock import patch

import go_test_shards as runner


class ShardTests(unittest.TestCase):
    """ShardTests cover the runner's safety properties without requiring a Go toolchain."""

    def test_partition_is_complete_disjoint_and_deterministic(self) -> None:
        """test_partition_is_complete_disjoint_and_deterministic prevents scheduling omissions."""
        names = [f"TestExample{i}" for i in range(100)] + ["FuzzUUID", "Example"]
        weights = {"TestExample0": 300, "TestExample1": 250, "DeletedTest": 9999}
        groups = runner.partition_tests(names, weights)
        self.assertEqual(groups, runner.partition_tests(list(reversed(names)), weights))
        flat = [name for group in groups for name in group]
        self.assertCountEqual(flat, names)
        self.assertEqual(len(flat), len(set(flat)))
        self.assertTrue(all(groups))
        self.assertNotEqual(next(i for i, g in enumerate(groups) if "TestExample0" in g),
                            next(i for i, g in enumerate(groups) if "TestExample1" in g))

    def test_invalid_inventory_and_hints_fail(self) -> None:
        """test_invalid_inventory_and_hints_fail rejects silent empty or malformed selections."""
        for names in ([], ["TestA", "TestA"], ["TestA/subtest"], ["TestA|TestB"]):
            with self.subTest(names=names), self.assertRaises(ValueError):
                runner.partition_tests(names, {})
        for weight in (-1, float("inf"), float("nan")):
            with self.subTest(weight=weight), self.assertRaises(ValueError):
                runner.partition_tests(["TestA"], {"TestA": weight})

    def test_selector_is_anchored_and_keeps_subtests(self) -> None:
        """test_selector_is_anchored_and_keeps_subtests protects names with common prefixes."""
        regex = runner.selector(["TestA", "TestAB", "FuzzInput", "Example"])
        self.assertIsNotNone(re.fullmatch(regex, "TestA"))
        self.assertIsNone(re.fullmatch(regex, "TestABC"))
        self.assertNotIn("/", regex)
        with self.assertRaises(ValueError):
            runner.selector([])

    def model_manifest(self, names: list[str]) -> dict:
        """model_manifest returns a minimal selected model inventory for event tests."""
        return {"shard": "model-0", "model_package": "example/model",
                "selected_packages": ["example/model"], "selected_tests": names}

    def model_events(self, names: list[str]) -> list[dict]:
        """model_events returns successful Go JSON events including nested test output."""
        events = []
        for name in names:
            events.extend([{"Action": "run", "Package": "example/model", "Test": name},
                           {"Action": "run", "Package": "example/model", "Test": name + "/child"},
                           {"Action": "pass", "Package": "example/model", "Test": name + "/child"},
                           {"Action": "pass", "Package": "example/model", "Test": name}])
        events.append({"Action": "pass", "Package": "example/model"})
        return events

    def test_events_detect_missing_duplicate_failed_and_unfinished_tests(self) -> None:
        """test_events_detect_missing_duplicate_failed_and_unfinished_tests tests actual output gates."""
        manifest = self.model_manifest(["TestA", "FuzzUUID", "Example"])
        events = self.model_events(manifest["selected_tests"])
        runner.validate_events(manifest, events)
        scenarios = [events[:-1], events + [events[0]], events + [events[-2]],
                     events[1:], events + [{"Action": "fail", "Package": "example/model", "Test": "TestA/child"}],
                     events + [{"Action": "build-fail", "ImportPath": "example/other"}],
                     self.model_events(["TestA", "Example"])]
        for changed in scenarios:
            with self.subTest(changed=changed), self.assertRaises(ValueError):
                runner.validate_events(manifest, changed)

    def test_no_test_packages_and_explicit_skips_are_accounted(self) -> None:
        """test_no_test_packages_and_explicit_skips_are_accounted preserves Go's normal semantics."""
        manifest = {"shard": "packages", "model_package": "example/model",
                    "selected_packages": ["example/empty", "example/tested"], "selected_tests": []}
        runner.validate_events(manifest, [{"Action": "skip", "Package": "example/empty"},
                                          {"Action": "pass", "Package": "example/tested"}])
        model = self.model_manifest(["TestOptIn"])
        runner.validate_events(model, [{"Action": "run", "Package": "example/model", "Test": "TestOptIn"},
                                       {"Action": "skip", "Package": "example/model", "Test": "TestOptIn"},
                                       {"Action": "pass", "Package": "example/model"}])

    def test_json_rejects_empty_truncated_and_malformed_logs(self) -> None:
        """test_json_rejects_empty_truncated_and_malformed_logs blocks misleading evidence."""
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "events.jsonl"
            for text in ("", '{"Action":', "not JSON\n", "[]\n", '{}\n'):
                path.write_text(text)
                with self.subTest(text=text), self.assertRaises(ValueError):
                    runner.read_events(path)

    def test_atomic_coverage_merge_deduplicates_statements(self) -> None:
        """test_atomic_coverage_merge_deduplicates_statements prevents inflated model coverage."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            first, second, output = [root / name for name in ("first", "second", "merged")]
            first.write_text("mode: atomic\nexample/model/a.go:1.1,2.2 2 1\nexample/model/a.go:3.1,4.2 1 0\n")
            second.write_text("mode: atomic\nexample/model/a.go:1.1,2.2 2 4\nexample/model/a.go:3.1,4.2 1 2\n")
            runner.merge_coverage([first, second], output)
            self.assertEqual(output.read_text(), "mode: atomic\nexample/model/a.go:1.1,2.2 2 5\nexample/model/a.go:3.1,4.2 1 2\n")

    def test_coverage_rejects_missing_empty_conflicting_or_wrong_mode(self) -> None:
        """test_coverage_rejects_missing_empty_conflicting_or_wrong_mode checks fail-closed merging."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            a, b, output = [root / name for name in ("a", "b", "out")]
            a.write_text("mode: atomic\nexample/a.go:1.1,2.2 2 1\n")
            for text in ("mode: count\nexample/a.go:1.1,2.2 2 1\n", "mode: atomic\n",
                         "mode: atomic\nexample/a.go:1.1,2.2 3 1\n", "mode: atomic\nbad\n",
                         "mode: atomic\nexample/a.go:1.1,2.2 2 1\nexample/a.go:1.1,2.2 2 1\n"):
                b.write_text(text)
                with self.subTest(text=text), self.assertRaises(ValueError):
                    runner.merge_coverage([a, b], output)
            with self.assertRaises(OSError):
                runner.merge_coverage([root / "missing"], output)

    def fixture_artifacts(self, root: Path) -> None:
        """fixture_artifacts builds a complete set of five independently checked shard artifacts."""
        for i, shard in enumerate(runner.SHARDS):
            path = root / shard
            path.mkdir()
            model = shard != "packages"
            names = [f"Test{i}"] if model else []
            manifest = {"shard": shard, "revision": "local", "exit_code": 0,
                        "all_packages": ["example/model", "example/other"],
                        "model_package": "example/model", "selected_tests": names,
                        "model_inventory": ["Test1", "Test2", "Test3", "Test4"] if model else [],
                        "selected_packages": ["example/model"] if model else ["example/other"]}
            (path / "manifest.json").write_text(json.dumps(manifest))
            events = self.model_events(names) if model else [{"Action": "pass", "Package": "example/other"}]
            (path / "events.jsonl").write_text("".join(json.dumps(event) + "\n" for event in events))
            package = "model" if model else "other"
            (path / "coverage.txt").write_text(f"mode: atomic\nexample/{package}/a.go:1.1,2.2 1 1\n")

    @patch.dict(os.environ, {"GITHUB_SHA": "local"})
    def test_merge_checks_all_shards_and_complete_inventory(self) -> None:
        """test_merge_checks_all_shards_and_complete_inventory exercises end-to-end reconciliation."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture_artifacts(root)
            runner.merge_shards(root, root / "coverage.txt")
            target = root / "model-0" / "manifest.json"
            original = json.loads(target.read_text())
            scenarios = [{"exit_code": 1}, {"revision": "other"}, {"shard": "model-1"},
                         {"selected_tests": []}, {"model_inventory": ["Test1"]},
                         {"all_packages": ["example/model"]}, {"selected_packages": []}]
            for change in scenarios:
                target.write_text(json.dumps({**original, **change}))
                with self.subTest(change=change), self.assertRaises(ValueError):
                    runner.merge_shards(root, root / "coverage.txt")
            target.unlink()
            with self.assertRaises(ValueError):
                runner.merge_shards(root, root / "coverage.txt")

    def test_discovery_uses_current_packages_and_names(self) -> None:
        """test_discovery_uses_current_packages_and_names verifies new tests are included automatically."""
        packages = "example/other\nexample/model\nexample/new\n"
        with patch.object(runner, "go_output", side_effect=[packages, "example", "TestA\nTestB\nFuzzA\nExample\nok example/model\n"]):
            plan = runner.discover("model-0")
        self.assertEqual(plan["all_packages"], ["example/model", "example/new", "example/other"])
        self.assertEqual(plan["model_inventory"], ["Example", "FuzzA", "TestA", "TestB"])
        with patch.object(runner, "go_output", side_effect=[packages, "example"]):
            self.assertEqual(runner.discover("packages")["selected_packages"], ["example/new", "example/other"])

    def test_summary_avoids_double_counting_subtests_and_escapes_names(self) -> None:
        """test_summary_avoids_double_counting_subtests_and_escapes_names protects readable reports."""
        events = self.model_events(["TestA"])
        events[-2]["Elapsed"] = 3
        events.insert(0, {"Action": "skip", "Package": "example/model", "Test": "TestB/<tag>|x"})
        summary = runner.summarize(events, "model-0", 5.0, 0)
        self.assertIn("1 passed, 0 failed, 0 skipped", summary)
        self.assertIn("&lt;tag&gt;&#124;x", summary)
        self.assertIn("Skipped tests (including subtests)", summary)

    def test_runner_preserves_nonzero_go_exit(self) -> None:
        """test_runner_preserves_nonzero_go_exit rejects green JSON from a failed Go process."""
        class FakeProcess:
            """FakeProcess emits plausible JSON while simulating a failed test command."""
            def __init__(self, *args, **kwargs):
                """__init__ prepares the JSON stream without executing Go."""
                self.stdout = iter([json.dumps({"Action": "pass", "Package": "example/other"}) + "\n"])
            def __enter__(self):
                """__enter__ returns the fake process."""
                return self
            def __exit__(self, *args):
                """__exit__ leaves the test fixture unchanged."""
                return None
            def wait(self):
                """wait returns the simulated Go failure status."""
                return 7
        manifest = {"shard": "packages", "revision": "local", "all_packages": ["example/other"],
                    "model_package": "example/model", "selected_packages": ["example/other"], "selected_tests": []}
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "coverage.txt").write_text("mode: atomic\nexample/other/a.go:1.1,2.2 1 1\n")
            with patch.object(runner, "discover", return_value=manifest), patch.object(
                    runner.subprocess, "Popen", FakeProcess), patch.dict(os.environ, {"GITHUB_STEP_SUMMARY": ""}):
                self.assertEqual(runner.run_shard("packages", root), 7)
            self.assertEqual(json.loads((root / "manifest.json").read_text())["exit_code"], 7)


if __name__ == "__main__":
    unittest.main(verbosity=2)
