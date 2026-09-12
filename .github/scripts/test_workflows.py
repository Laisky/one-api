#!/usr/bin/env python3
"""Test workflow contracts and execute the CI gate against representative job results.

Run with Python 3 and PyYAML 6.0.3. GitHub expression/schema validation is also
performed by actionlint in the Go job; these tests protect repository behavior.
"""

from __future__ import annotations

import copy
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = ROOT / ".github" / "workflows"
THEMES = ("modern", "air", "berry")
MANDATORY = {
    "changes", "go_tests", "goroutine_context_guard",
    "entity_response_guard", "vulnerability_scan",
}


class UniqueKeyLoader(yaml.BaseLoader):
    """UniqueKeyLoader rejects duplicate keys and preserves GitHub's on keyword."""

    def construct_mapping(self, node: yaml.MappingNode, deep: bool = False) -> dict:
        """construct_mapping returns a mapping or rejects a duplicate YAML key."""
        result = {}
        for key_node, value_node in node.value:
            key = self.construct_object(key_node, deep=deep)
            if key in result:
                raise ValueError(f"Duplicate YAML key: {key}")
            result[key] = self.construct_object(value_node, deep=deep)
        return result


def load_workflow(name: str) -> dict:
    """load_workflow returns a parsed workflow without YAML 1.1 boolean coercion."""
    return yaml.load((WORKFLOWS / name).read_text(), Loader=UniqueKeyLoader)


def commands(job: dict) -> str:
    """commands returns all shell commands in a job for contract assertions."""
    return "\n".join(step.get("run", "") for step in job.get("steps", []))


class WorkflowTests(unittest.TestCase):
    """WorkflowTests protect coverage, trust boundaries and delivery behavior."""

    @classmethod
    def setUpClass(cls) -> None:
        """setUpClass loads the two maintained workflow definitions."""
        cls.ci = load_workflow("lint.yml")
        cls.delivery = load_workflow("ci.yml")

    def test_only_two_workflows(self) -> None:
        """test_only_two_workflows rejects reintroduced one-off CI entrypoints."""
        files = {p.name for p in WORKFLOWS.iterdir() if p.suffix in {".yml", ".yaml"}}
        self.assertEqual(files, {"ci.yml", "lint.yml"})

    def test_ci_events_are_not_path_filtered(self) -> None:
        """test_ci_events_are_not_path_filtered keeps the required check reachable."""
        self.assertEqual(set(self.ci["on"]), {
            "push", "pull_request", "merge_group", "workflow_dispatch",
        })
        self.assertEqual(self.ci["on"]["push"]["branches"], ["master", "main", "test/ci"])
        for event in ("push", "pull_request", "merge_group"):
            trigger = self.ci["on"][event] or {}
            self.assertNotIn("paths", trigger)
            self.assertNotIn("paths-ignore", trigger)
        self.assertEqual(self.ci["concurrency"]["cancel-in-progress"], "true")
        self.assertEqual(self.ci["on"]["workflow_dispatch"]["inputs"]["historical_control"]["default"], "false")

    def test_full_go_suite_runs_once_without_path_skips(self) -> None:
        """test_full_go_suite_runs_once_without_path_skips protects full race coverage."""
        all_commands = "\n".join(commands(job) for job in self.ci["jobs"].values())
        full_suites = re.findall(r"go test[^\n]*\./\.\.\.", all_commands)
        self.assertEqual(len(full_suites), 1)
        for flag in ("-race", "-count=1", "-timeout 45m", "-coverprofile=coverage.txt", "-v"):
            self.assertIn(flag, full_suites[0])
        self.assertEqual(all_commands.count("go vet ./..."), 1)
        job = self.ci["jobs"]["go_tests"]
        self.assertNotIn("if", job)
        self.assertGreaterEqual(int(job["timeout-minutes"]), 90)
        self.assertEqual(job["steps"][0]["with"]["fetch-depth"], "0")
        self.assertIn('tee "$RUNNER_TEMP/go-tests.log"', commands(job))
        self.assertIn("actionlint@v1.7.12", commands(job))
        for step in job["steps"]:
            self.assertNotIn("continue-on-error", step)

    def test_database_qualification_cannot_silently_skip(self) -> None:
        """test_database_qualification_cannot_silently_skip protects live UUID tests."""
        job = self.ci["jobs"]["go_tests"]
        self.assertEqual(job["services"]["mysql"]["image"], "mysql:8.4")
        self.assertEqual(job["services"]["postgres"]["image"], "postgres:17")
        env = job["env"]
        for guard in ("ONEAPI_REQUIRE_DB_BACKENDS", "ONEAPI_REQUIRE_COMPACT_UUID_SUITE", "CGO_ENABLED"):
            self.assertEqual(env[guard], "1")
        for dsn in ("MYSQL_DSN", "PG_DSN", "MYSQL_LOG_DSN", "PG_LOG_DSN",
                    "COMPACT_UUID_TEST_MYSQL_DSN", "COMPACT_UUID_TEST_POSTGRES_DSN",
                    "COMPACT_UUID_TEST_LOG_MYSQL_DSN", "COMPACT_UUID_TEST_LOG_POSTGRES_DSN",
                    "COMPACT_UUID_TEST_POSTGRES_BASE_DSN"):
            self.assertTrue(env[dsn], dsn)
        for dependency in ("ffmpeg", "mysql-client", "postgresql-client", "oneapi_log", "oneapi_workload_base"):
            self.assertIn(dependency, commands(job))

    def test_static_and_security_guards_remain(self) -> None:
        """test_static_and_security_guards_remain protects non-test validation."""
        jobs = self.ci["jobs"]
        for name in MANDATORY - {"changes", "go_tests"}:
            self.assertNotIn("if", jobs[name])
        self.assertIn("ast-grep test --skip-snapshot-tests", commands(jobs["goroutine_context_guard"]))
        self.assertIn("ast-grep scan", commands(jobs["goroutine_context_guard"]))
        self.assertIn("sha256sum -c -", commands(jobs["goroutine_context_guard"]))
        self.assertIn("./tools/analyzers/noentityresponse/cmd/noentityresponse ./...", commands(jobs["entity_response_guard"]))
        self.assertIn("--enable-only err113", commands(jobs["entity_response_guard"]))
        self.assertIn('"$(go env GOPATH)/bin/govulncheck" ./...', commands(jobs["vulnerability_scan"]))

    def test_frontend_selection_and_frozen_build(self) -> None:
        """test_frontend_selection_and_frozen_build protects tests and Modern build."""
        changes = self.ci["jobs"]["changes"]
        self.assertEqual(changes["permissions"]["pull-requests"], "read")
        filter_step = next(step for step in changes["steps"] if step.get("id") == "filter")
        self.assertIn("github.event_name == 'push' && github.ref", filter_step["with"]["base"])
        filters = yaml.load(filter_step["with"]["filters"], Loader=UniqueKeyLoader)
        for theme in THEMES:
            for path in (f"web/{theme}/**", ".github/**", "Makefile", "Dockerfile", ".dockerignore", "VERSION"):
                self.assertIn(path, filters[theme])
            job = self.ci["jobs"][f"{theme}_frontend_tests"]
            self.assertIn("workflow_dispatch", job["if"])
            self.assertIn(f"needs.changes.outputs.{theme} == 'true'", job["if"])
            self.assertIn("yarn install --frozen-lockfile", commands(job))
            self.assertIn("yarn test", commands(job))
        self.assertIn("yarn build", commands(self.ci["jobs"]["modern_frontend_tests"]))

    def test_read_only_validation_and_no_checkout_credentials(self) -> None:
        """test_read_only_validation_and_no_checkout_credentials protects PR execution."""
        self.assertEqual(self.ci["permissions"], {"contents": "read"})
        self.assertNotIn("pull_request_target", self.ci["on"])
        self.assertNotIn("secrets.", (WORKFLOWS / "lint.yml").read_text())
        for name, job in self.ci["jobs"].items():
            for permission in job.get("permissions", {}).values():
                if name != "code_coverage":
                    self.assertNotEqual(permission, "write")
            for step in job.get("steps", []):
                if step.get("uses", "").startswith("actions/checkout@"):
                    self.assertEqual(step["with"]["persist-credentials"], "false")
        reporter = self.ci["jobs"]["code_coverage"]
        self.assertIn("head.repo.full_name == github.repository", reporter["if"])
        self.assertIn("github.actor != 'dependabot[bot]'", reporter["if"])
        self.assertNotIn("code_coverage", self.ci["jobs"]["required"]["needs"])

    def test_historical_replay_is_manual_and_still_checks_the_assertion(self) -> None:
        """test_historical_replay_is_manual_and_still_checks_the_assertion preserves evidence."""
        job = self.ci["jobs"]["historical_control"]
        self.assertEqual(job["if"], "github.event_name == 'workflow_dispatch' && inputs.historical_control")
        refs = [step.get("with", {}).get("ref") for step in job["steps"]]
        self.assertIn("6138a94f9749312a9c31c7c2ebdcbea758b04235", refs)
        self.assertIn('"$status" -ne 1', commands(job))
        self.assertIn("--- FAIL: TestReviewTranscriptionIndexPresence", commands(job))

    def test_delivery_keeps_existing_triggers_tags_and_dependency(self) -> None:
        """test_delivery_keeps_existing_triggers_tags_and_dependency prevents PR deployment."""
        self.assertEqual(set(self.delivery["on"]), {"push"})
        trigger = self.delivery["on"]["push"]
        self.assertEqual(trigger["branches"], ["master", "main", "test/ci"])
        self.assertEqual(trigger["paths-ignore"], ["docs/**", "doc/**", "tests/**", "test/**", "ci/**"])
        jobs = self.delivery["jobs"]
        self.assertEqual(set(jobs), {"check_skip", "build_latest", "build_arm64_hash", "deploy"})
        self.assertEqual(jobs["deploy"]["needs"], ["check_skip", "build_latest"])
        for job_name, prefix in (("build_latest", ""), ("build_arm64_hash", "arm64-")):
            build = next(step["with"] for step in jobs[job_name]["steps"] if step.get("uses", "").startswith("docker/build-push-action@"))
            self.assertEqual(build["tags"].splitlines(), [
                f"ppcelery/one-api:{prefix}latest",
                "ppcelery/one-api:" + prefix + "${{ steps.vars.outputs.short_sha }}",
            ])
            scope = "one-api-arm64" if prefix else "one-api-amd64"
            self.assertIn(f"scope={scope}", build["cache-from"])
            self.assertIn(f"scope={scope}", build["cache-to"])
            if not prefix:
                self.assertIn("ignore-error=true", build["cache-to"])

    def test_shell_blocks_parse(self) -> None:
        """test_shell_blocks_parse validates shell syntax without executing workflow actions."""
        for workflow in (self.ci, self.delivery):
            for job in workflow["jobs"].values():
                for step in job.get("steps", []):
                    if "run" in step:
                        script = re.sub(r"\$\{\{.*?\}\}", "expression", step["run"])
                        result = subprocess.run(["bash", "-n"], input=script, text=True, capture_output=True, check=False)
                        self.assertEqual(result.returncode, 0, f"{step.get('name')}: {result.stderr}")

    def test_required_gate_success_failure_cancellation_and_skips(self) -> None:
        """test_required_gate_success_failure_cancellation_and_skips executes the actual gate."""
        gate = self.ci["jobs"]["required"]
        self.assertEqual(gate["if"], "always()")
        expected = MANDATORY | {f"{theme}_frontend_tests" for theme in THEMES} | {"historical_control"}
        self.assertEqual(set(gate["needs"]), expected)
        script = gate["steps"][0]["run"].split("<<'PY'\n", 1)[1].rsplit("\nPY", 1)[0]
        baseline = {name: {"result": "success"} for name in expected}
        baseline["changes"]["outputs"] = {theme: "false" for theme in THEMES}
        for name in expected - MANDATORY:
            baseline[name]["result"] = "skipped"
        scenarios = [("unaffected frontend skips", baseline, False, False, True)]
        for name in expected:
            for status in ("failure", "cancelled"):
                state = copy.deepcopy(baseline)
                state[name]["result"] = status
                scenarios.append((f"{name} {status}", state, False, False, False))
        for name in MANDATORY:
            state = copy.deepcopy(baseline)
            state[name]["result"] = "skipped"
            scenarios.append((f"mandatory {name} skipped", state, False, False, False))
        for name in MANDATORY:
            state = copy.deepcopy(baseline)
            del state[name]
            scenarios.append((f"mandatory {name} missing", state, False, False, False))
        for theme in THEMES:
            state = copy.deepcopy(baseline)
            state["changes"]["outputs"][theme] = "true"
            scenarios.append((f"affected {theme} skipped", state, False, False, False))
            state = copy.deepcopy(state)
            state[f"{theme}_frontend_tests"]["result"] = "success"
            scenarios.append((f"affected {theme} passed", state, False, False, True))
        state = copy.deepcopy(baseline)
        state["changes"]["outputs"] = {}
        scenarios.append(("missing change outputs", state, False, False, False))
        scenarios.append(("manual skips not allowed", baseline, True, False, False))
        state = copy.deepcopy(baseline)
        for theme in THEMES:
            state[f"{theme}_frontend_tests"]["result"] = "success"
        scenarios.append(("manual all frontends", state, True, False, True))
        scenarios.append(("requested history skipped", state, True, True, False))
        state = copy.deepcopy(state)
        state["historical_control"]["result"] = "success"
        scenarios.append(("requested history passed", state, True, True, True))
        for label, jobs, manual, historical, success in scenarios:
            with self.subTest(label=label):
                env = {**os.environ, "NEEDS": json.dumps(jobs),
                       "MANUAL_RUN": str(manual).lower(), "HISTORICAL_CONTROL": str(historical).lower()}
                result = subprocess.run([sys.executable, "-c", script], env=env, text=True, capture_output=True, check=False)
                self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main(verbosity=2)
