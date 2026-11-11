#!/usr/bin/env python3
"""Test workflow coverage, trust boundaries and the actual aggregate status gate."""
from __future__ import annotations

import copy
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

import yaml

import go_test_shards as shards

ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = ROOT / ".github" / "workflows"
sys.path.insert(0, str(ROOT / "scripts"))
from image_inputs import read_image_inputs
THEMES = ("modern", "air", "berry")
MANDATORY = {
    "changes", "quick_tests", "goroutine_context_guard",
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
    """load_workflow returns YAML without coercing GitHub's on keyword to a boolean."""
    return yaml.load((WORKFLOWS / name).read_text(), Loader=UniqueKeyLoader)


def commands(job: dict) -> str:
    """commands returns all shell commands in a job for contract assertions."""
    return "\n".join(step.get("run", "") for step in job.get("steps", []))


class WorkflowTests(unittest.TestCase):
    """WorkflowTests protect validation and the safeguarded delivery policy."""

    @classmethod
    def setUpClass(cls) -> None:
        """setUpClass loads the two maintained workflow definitions."""
        cls.ci = load_workflow("lint.yml")
        cls.delivery = load_workflow("ci.yml")

    def test_only_two_workflows(self) -> None:
        """test_only_two_workflows rejects reintroduced one-off CI entrypoints."""
        files = {p.name for p in WORKFLOWS.iterdir() if p.suffix in {".yml", ".yaml"}}
        self.assertEqual(files, {"ci.yml", "lint.yml"})

    def test_kubernetes_gateway_documentation_contracts(self) -> None:
        """test_kubernetes_gateway_documentation_contracts runs the shipped guide regressions."""
        result = subprocess.run(
            [sys.executable, "-m", "unittest", "discover", "-s", str(ROOT / "scripts"),
             "-p", "test_k8s_gateway_docs.py", "-v"],
            cwd=ROOT, capture_output=True, text=True, timeout=120, check=False,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_ci_events_are_not_path_filtered(self) -> None:
        """test_ci_events_are_not_path_filtered keeps the required check reachable."""
        self.assertEqual(set(self.ci["on"]), {"push", "pull_request", "merge_group", "workflow_dispatch"})
        self.assertEqual(self.ci["on"]["push"]["branches"], ["master", "main", "test/ci"])
        for event in ("push", "pull_request", "merge_group"):
            trigger = self.ci["on"][event] or {}
            self.assertNotIn("paths", trigger)
            self.assertNotIn("paths-ignore", trigger)
        self.assertEqual(self.ci["concurrency"]["cancel-in-progress"], "true")
        self.assertEqual(self.ci["on"]["workflow_dispatch"]["inputs"]["historical_control"]["default"], "false")

    def test_go_validation_uses_security_baseline(self) -> None:
        """test_go_validation_uses_security_baseline pins CI, module and image compilers."""
        expected = {"quick_tests", "go_test_shards", "entity_response_guard",
                    "vulnerability_scan", "historical_control", "native_arm64_images", "native_amd64_images"}
        setups = {
            name: [step for step in job.get("steps", [])
                   if step.get("uses", "").startswith("actions/setup-go@")]
            for name, job in self.ci["jobs"].items()
        }
        self.assertEqual({name for name, steps in setups.items() if steps}, expected)
        for name in expected:
            self.assertEqual(len(setups[name]), 1, name)
            wanted = {"go-version": "1.27.4"}
            if name in {"native_arm64_images", "native_amd64_images"}:
                wanted["cache"] = "false"
            self.assertEqual(setups[name][0]["with"], wanted, name)
        self.assertRegex((ROOT / "go.mod").read_text(), r"(?m)^go 1\.27\.4$")
        self.assertIn("ARG GO_IMAGE=golang:1.27.4-bookworm", (ROOT / "Dockerfile").read_text())

    def test_go_shards_preserve_fresh_race_coverage_and_complete_inventory(self) -> None:
        """test_go_shards_preserve_fresh_race_coverage_and_complete_inventory protects selection."""
        job = self.ci["jobs"]["go_test_shards"]
        self.assertEqual(job["if"], "github.event_name == 'workflow_dispatch' && inputs.qualification")
        self.assertGreaterEqual(int(job["timeout-minutes"]), 90)
        self.assertEqual(job["strategy"]["fail-fast"], "false")
        self.assertEqual(job["strategy"]["matrix"]["shard"], list(shards.SHARDS))
        self.assertEqual(job["steps"][0]["with"]["fetch-depth"], "0")
        for flag in ("-race", "-cover", "-covermode=atomic", "-count=1", "-timeout=45m"):
            self.assertIn(flag, shards.FLAGS)
        self.assertNotIn("-short", shards.FLAGS)
        self.assertIn("go_test_shards.py run", commands(job))
        self.assertIn('--shard "$GO_TEST_SHARD"', commands(job))
        evidence = next(step for step in job["steps"] if step.get("uses", "").startswith("actions/upload-artifact@"))
        self.assertEqual(evidence["if"], "always()")
        self.assertIn("matrix.shard", evidence["with"]["name"])
        self.assertEqual(evidence["with"]["retention-days"], "14")
        aggregate = self.ci["jobs"]["go_tests"]
        self.assertEqual(aggregate["needs"], "go_test_shards")
        self.assertEqual(aggregate["if"], "always() && github.event_name == 'workflow_dispatch' && inputs.qualification")
        self.assertIn("go_test_shards.py merge", commands(aggregate))
        download = next(step for step in aggregate["steps"] if step.get("uses", "").startswith("actions/download-artifact@"))
        self.assertEqual(download["with"]["pattern"], "go-tests-*-${{ github.sha }}")
        self.assertNotEqual(download["with"].get("merge-multiple"), "true")
        artifact = aggregate["steps"][-1]["with"]
        self.assertEqual(artifact["name"], "code-coverage")
        self.assertEqual(artifact["if-no-files-found"], "error")
        self.assertNotIn("code_coverage", self.ci["jobs"])
        for candidate in (job, aggregate):
            self.assertNotIn("continue-on-error", candidate)
            for step in candidate["steps"]:
                self.assertNotIn("continue-on-error", step)

    def test_matrix_failure_cannot_publish_coverage(self) -> None:
        """test_matrix_failure_cannot_publish_coverage executes the aggregation precondition."""
        step = self.ci["jobs"]["go_tests"]["steps"][0]
        self.assertEqual(step["env"]["SHARD_RESULT"], "${{ needs.go_test_shards.result }}")
        for status in ("success", "failure", "cancelled", "skipped", ""):
            result = subprocess.run(["bash", "-e", "-c", step["run"]],
                                    env={**os.environ, "SHARD_RESULT": status}, check=False)
            self.assertEqual(result.returncode == 0, status == "success", status)

    def test_database_qualification_cannot_silently_skip(self) -> None:
        """test_database_qualification_cannot_silently_skip protects live UUID tests on every shard."""
        job = self.ci["jobs"]["go_test_shards"]
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
        """test_static_and_security_guards_remain protects non-test validation without duplication."""
        jobs = self.ci["jobs"]
        for name in MANDATORY - {"changes", "quick_tests"}:
            self.assertNotIn("if", jobs[name])
        self.assertIn("ast-grep test --skip-snapshot-tests", commands(jobs["goroutine_context_guard"]))
        self.assertIn("ast-grep scan", commands(jobs["goroutine_context_guard"]))
        self.assertIn("sha256sum -c -", commands(jobs["goroutine_context_guard"]))
        static = commands(jobs["entity_response_guard"])
        self.assertIn("./tools/analyzers/noentityresponse/cmd/noentityresponse ./...", static)
        self.assertIn("--enable-only err113", static)
        sentinel = next(step for step in jobs["entity_response_guard"]["steps"]
                        if step.get("name") == "Check realtime sentinel error convention")
        self.assertEqual(sentinel["run"],
                         "go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 "
                         "run --no-config --enable-only err113 --timeout=5m ./relay/realtime")
        self.assertIn("actionlint@v1.7.12", static)
        self.assertIn("go vet ./...", static)
        self.assertEqual("\n".join(commands(job) for job in jobs.values()).count("go vet ./..."), 1)
        self.assertIn('"$(go env GOPATH)/bin/govulncheck" ./...', commands(jobs["vulnerability_scan"]))

    def test_frontend_selection_and_frozen_build(self) -> None:
        """test_frontend_selection_and_frozen_build protects tests and Modern production build."""
        changes = self.ci["jobs"]["changes"]
        self.assertEqual(changes["permissions"]["pull-requests"], "read")
        filter_step = next(step for step in changes["steps"] if step.get("id") == "filter")
        self.assertIn("github.event_name == 'push' && github.ref", filter_step["with"]["base"])
        filters = yaml.load(filter_step["with"]["filters"], Loader=UniqueKeyLoader)
        for theme in THEMES:
            for path in (f"web/{theme}/**", ".github/**", "Makefile", "Dockerfile", ".dockerignore", "VERSION"):
                self.assertIn(path, filters[theme])
            job = self.ci["jobs"][f"{theme}_frontend_tests"]
            self.assertEqual(job["if"], "github.event_name == 'workflow_dispatch' && inputs.qualification")
            audit = self.ci["jobs"][f"{theme}_dependency_audit"]
            self.assertIn(f"needs.changes.outputs.{theme} == 'true'", audit["if"])
            self.assertIn("workflow_dispatch", audit["if"])
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
                self.assertNotEqual(permission, "write")
            for step in job.get("steps", []):
                if step.get("uses", "").startswith("actions/checkout@"):
                    self.assertEqual(step["with"]["persist-credentials"], "false")

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

    def test_delivery_uses_the_qualified_dockerfile_inputs(self) -> None:
        """test_delivery_uses_the_qualified_dockerfile_inputs rejects source or image overrides in both builds."""
        defaults = read_image_inputs((ROOT/"Dockerfile").read_text(), "1.27.4")
        def effective(arguments):
            """effective applies Docker build arguments to the real canonical defaults."""
            result = dict(defaults)
            for line in arguments.splitlines():
                line = line.strip()
                if not line:
                    continue
                key, value = line.split("=", 1)
                if key.strip() in result:
                    result[key.strip()] = value.strip()
            return result
        for name in ("build_latest", "build_arm64_hash"):
            job = self.delivery["jobs"][name]
            checkout = next(step for step in job["steps"] if step.get("uses", "").startswith("actions/checkout@"))
            self.assertEqual(checkout.get("with", {}).get("ref", "${{ github.sha }}"), "${{ github.sha }}")
            build = next(step["with"] for step in job["steps"] if step.get("uses", "").startswith("docker/build-push-action@"))
            self.assertEqual(build["context"], ".")
            self.assertEqual(build.get("file", "Dockerfile"), "Dockerfile")
            self.assertEqual(effective(build.get("build-args", "")), defaults)
        self.assertEqual(effective("  TARGETARCH=arm64\n\n"), defaults)
        for key, pin in defaults.items():
            self.assertEqual(effective(key+"="+pin), defaults)
            for value in (pin.split("@")[0], pin[:-64]+"f"*64, "${{ env.UNREVIEWED_IMAGE }}"):
                with self.subTest(key=key, value=value):
                    for prefix in ("", "  ", "\t"):
                        self.assertNotEqual(effective(prefix+key+"="+value), defaults)
        filters = commands(self.ci["jobs"]["changes"])
        self.assertIn("python3 scripts/test_image_inputs.py", filters)

    def test_delivery_revision_label_matches_source_on_both_architectures(self) -> None:
        """test_delivery_revision_label_matches_source_on_both_architectures binds each image to its source."""
        for name in ("build_latest", "build_arm64_hash"):
            with self.subTest(job=name):
                build = next(step["with"] for step in self.delivery["jobs"][name]["steps"]
                             if step.get("uses", "").startswith("docker/build-push-action@"))
                self.assertEqual(build.get("labels"),
                                 "org.opencontainers.image.revision=${{ github.sha }}")

    @unittest.skipUnless(os.environ.get("ONEAPI_TEST_IMAGE_LABELS") == "1",
                         "opt in to disposable local Docker metadata builds")
    def test_delivery_revision_label_overrides_inherited_base_metadata(self) -> None:
        """test_delivery_revision_label_overrides_inherited_base_metadata builds only local metadata fixtures."""
        import fcntl
        import socket
        import uuid

        with open("/tmp/laisky-heavy-validation-20261008.lock", "a") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            self.assertEqual(socket.gethostname(), "dev", "Metadata fixtures require the dev host")
            for name in ("DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH"):
                self.assertFalse(os.environ.get(name), f"Unset {name} before using the local daemon")
            env = {**os.environ, "DOCKER_BUILDKIT": "0"}

            def docker(*args: str, source: str | None = None) -> str:
                """docker executes local fixture operations and returns their standard output."""
                result = subprocess.run(["docker", *args], input=source, text=True,
                                        capture_output=True, env=env, timeout=90, check=False)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                return result.stdout.strip()

            self.assertEqual(docker("context", "show"), "default")
            endpoint = json.loads(docker("context", "inspect", "--format", "{{json .Endpoints.docker.Host}}"))
            self.assertEqual(endpoint, "unix:///var/run/docker.sock")
            self.assertEqual(json.loads(docker("info", "--format", "{{json .Name}}")), "dev")
            base = f"oneapi-label-fixture-{uuid.uuid4().hex}:base"
            stale, release = "b" * 40, "a" * 40
            key = "org.opencontainers.image.revision"
            tags = []

            def remove_fixtures() -> None:
                """remove_fixtures removes only successfully created unique fixture tags."""
                if tags:
                    docker("image", "rm", *reversed(tags))

            self.addCleanup(remove_fixtures)
            docker("build", "--network=none", "--pull=false", "--tag", base, "-",
                   source=f'FROM scratch\nLABEL {key}="{stale}"\n')
            tags.append(base)
            self.assertEqual(json.loads(docker("image", "inspect", "--format",
                                              "{{json .Config.Labels}}", base))[key], stale)
            control = base.replace(":base", ":unlabeled-control")
            docker("build", "--network=none", "--pull=false", "--tag", control, "-",
                   source=f"FROM {base}\n")
            tags.append(control)
            self.assertEqual(json.loads(docker("image", "inspect", "--format",
                                              "{{json .Config.Labels}}", control))[key], stale)
            for name in ("build_latest", "build_arm64_hash"):
                with self.subTest(job=name):
                    build = next(step["with"] for step in self.delivery["jobs"][name]["steps"]
                                 if step.get("uses", "").startswith("docker/build-push-action@"))
                    child = base.replace(":base", ":" + name.replace("_", "-"))
                    args = ["build", "--network=none", "--pull=false", "--tag", child]
                    for label in build.get("labels", "").splitlines():
                        args.extend(["--label", label.replace("${{ github.sha }}", release)])
                    docker(*args, "-", source=f"FROM {base}\n")
                    tags.append(child)
                    labels = json.loads(docker("image", "inspect", "--format",
                                               "{{json .Config.Labels}}", child))
                    print(json.dumps({"job": name, "base_revision": stale,
                                      "expected_revision": release, "actual_revision": labels[key]}))
                    self.assertEqual(labels[key], release,
                                     f"{name} inherited a base-image revision instead of source SHA")

    def test_delivery_uses_direct_compose(self) -> None:
        """test_delivery_uses_direct_compose preserves production scope without host Python."""
        deploy = self.delivery["jobs"]["deploy"]
        self.assertEqual(deploy["if"],
                         "needs.check_skip.outputs.should_skip != 'true' && github.ref == 'refs/heads/main'")
        self.assertEqual(deploy["concurrency"], {
            "group": "deploy-oneapi-b1", "cancel-in-progress": "false",
        })
        self.assertEqual(len(deploy["steps"]), 1)
        caller = deploy["steps"][0]
        self.assertTrue(caller["uses"].startswith("appleboy/ssh-action@"))
        self.assertEqual(caller["with"]["command_timeout"], "20m")
        self.assertGreater(int(deploy["timeout-minutes"]), 20)
        for key in ("key", "fingerprint", "host", "username", "port"):
            self.assertIn(key, caller["with"])
        script = caller["with"]["script"]
        for forbidden in ("python", "deploy_oneapi", "--remove-orphans", "down", "prune"):
            self.assertNotIn(forbidden, script)

    def test_remote_pull_recreate_and_failure_propagation(self) -> None:
        """test_remote_pull_recreate_and_failure_propagation runs the real caller with fake executables."""
        script = self.delivery["jobs"]["deploy"]["steps"][0]["with"]["script"]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            bootstrap = root / "bootstrap"
            bootstrap.write_text('export INJECTOR_BOOTSTRAPPED=yes\n')
            script = script.replace("/home/laisky/.zshrc", str(bootstrap))
            script = script.replace("/home/laisky/repo/VPS", directory)
            # A reverted workflow must never invoke a real host deployer during tests.
            retired = root / "deploy_oneapi.py"
            retired.write_text(">>>>>>> 9a3e2ec (fix)\n")
            script = script.replace("/opt/configs/observability/deploy_oneapi.py", str(retired))
            calls = root / "calls.jsonl"
            shim = (
                f"#!{sys.executable}\n"
                "import json, os, pathlib, sys\n"
                "name = pathlib.Path(sys.argv[0]).name\n"
                "args = sys.argv[1:]\n"
                "with open(os.environ['CALLS'], 'a') as out: out.write(json.dumps([name, *args]) + '\\n')\n"
                "if name == 'op':\n"
                "    assert os.environ.get('INJECTOR_BOOTSTRAPPED') == 'yes'\n"
                "    if os.environ['FAIL_AT'] == 'op': sys.exit(24)\n"
                "    command = args[args.index('--') + 1:]\n"
                "    os.execvp(command[0], command)\n"
                "if 'pull' in args and os.environ['FAIL_AT'] == 'pull': sys.exit(23)\n"
                "if 'up' in args and os.environ['FAIL_AT'] == 'up': sys.exit(25)\n"
            )
            for name in ("docker", "op"):
                executable = root / name
                executable.write_text(shim)
                executable.chmod(0o700)
            for failure, status in (("", 0), ("pull", 23), ("op", 24), ("up", 25)):
                with self.subTest(failure=failure):
                    calls.write_text("")
                    result = subprocess.run(["/bin/sh", "-c", script], capture_output=True,
                                            text=True, check=False, env={
                                                **os.environ, "PATH": directory + ":" + os.environ["PATH"],
                                                "CALLS": str(calls), "FAIL_AT": failure, "ZDOTDIR": directory,
                                                "RELEASE_SHA": "a" * 40,
                                                "IMAGE_DIGEST": "sha256:" + "b" * 64,
                                            })
                    self.assertEqual(result.returncode, status, result.stderr)
                    actual = [json.loads(line) for line in calls.read_text().splitlines()]
                    expected = [["docker", "pull", "ppcelery/one-api:latest"]]
                    compose = ["docker", "compose", "--compatibility",
                               "--env-file", "/opt/configs/observability/.env",
                               "-f", "b1-docker-compose.yml", "-f", "observability/one-api.override.yml",
                               "up", "-d", "--no-deps", "--force-recreate", "--timeout", "420", "oneapi"]
                    if failure != "pull":
                        expected.append(["op", "run", "--env-file=one-api/b1.env", "--", *compose])
                    if failure not in ("pull", "op"):
                        expected.append(compose)
                    self.assertEqual(actual, expected)
            bootstrap.unlink()
            calls.write_text("")
            result = subprocess.run(["/bin/sh", "-c", script], capture_output=True, text=True,
                                    check=False, env={**os.environ, "PATH": directory + ":" + os.environ["PATH"],
                                                      "CALLS": str(calls), "FAIL_AT": "", "ZDOTDIR": directory})
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(calls.read_text(), "", "Missing bootstrap must not reach Docker")

    def test_shell_blocks_parse(self) -> None:
        """test_shell_blocks_parse validates shell syntax without executing workflow actions."""
        for workflow in (self.ci, self.delivery):
            for job in workflow["jobs"].values():
                for step in job.get("steps", []):
                    scripts = [step["run"]] if "run" in step else []
                    if step.get("uses", "").startswith("appleboy/ssh-action@"):
                        scripts.append(step["with"]["script"])
                    for source in scripts:
                        script = re.sub(r"\$\{\{.*?\}\}", "expression", source)
                        result = subprocess.run(["bash", "-n"], input=script, text=True, capture_output=True, check=False)
                        self.assertEqual(result.returncode, 0, f"{step.get('name')}: {result.stderr}")

    def test_quick_gate_has_no_service_or_full_qualification_dependencies(self) -> None:
        """test_quick_gate_has_no_service_or_full_qualification_dependencies enforces the small gate."""
        job = self.ci["jobs"]["quick_tests"]
        self.assertNotIn("if", job)
        self.assertNotIn("services", job)
        self.assertNotIn("needs", job)
        self.assertLessEqual(int(job["timeout-minutes"]), 5)
        self.assertIn("format_check.py", commands(job))
        self.assertIn("quick_tests.py", commands(job))
        self.assertNotIn("-race", commands(job))
        self.assertNotIn("go_test_shards.py", commands(job))
        for name in ("go_test_shards", "modern_frontend_tests", "air_frontend_tests", "berry_frontend_tests"):
            self.assertEqual(self.ci["jobs"][name]["if"],
                             "github.event_name == 'workflow_dispatch' && inputs.qualification")
        self.assertEqual(self.ci["on"]["workflow_dispatch"]["inputs"]["qualification"]["default"], "false")

    def test_required_gate_success_failure_cancellation_and_skips(self) -> None:
        """test_required_gate_success_failure_cancellation_and_skips executes both policy modes."""
        gate = self.ci["jobs"]["required"]
        self.assertEqual(gate["if"], "always()")
        audits = {f"{theme}_dependency_audit" for theme in THEMES}
        frontends = {f"{theme}_frontend_tests" for theme in THEMES}
        expected = MANDATORY | audits | frontends | {"go_tests", "historical_control", "native_arm64_images", "native_amd64_images"}
        self.assertEqual(set(gate["needs"]), expected)
        script = gate["steps"][0]["run"].split("<<'PY'\n", 1)[1].rsplit("\nPY", 1)[0]
        baseline = {name: {"result": "success"} for name in expected}
        baseline["changes"]["outputs"] = {theme: "false" for theme in THEMES}
        for name in expected - MANDATORY:
            baseline[name]["result"] = "skipped"
        scenarios = [("unaffected manual suites and audits skipped", baseline, False, False, False, True)]
        for name in expected:
            for status in ("failure", "cancelled"):
                state = copy.deepcopy(baseline)
                state[name]["result"] = status
                scenarios.append((f"{name} {status}", state, False, False, False, False))
        for name in MANDATORY:
            state = copy.deepcopy(baseline)
            state[name]["result"] = "skipped"
            scenarios.append((f"mandatory {name} skipped", state, False, False, False, False))
            state = copy.deepcopy(baseline)
            del state[name]
            scenarios.append((f"mandatory {name} missing", state, False, False, False, False))
        for theme in THEMES:
            state = copy.deepcopy(baseline)
            state["changes"]["outputs"][theme] = "true"
            scenarios.append((f"affected {theme} audit skipped", state, False, False, False, False))
            state = copy.deepcopy(state)
            state[f"{theme}_dependency_audit"]["result"] = "success"
            scenarios.append((f"affected {theme} audit passed", state, False, False, False, True))
        state = copy.deepcopy(baseline)
        state["changes"]["outputs"] = {}
        scenarios.append(("missing change outputs", state, False, False, False, False))
        scenarios.append(("manual audits skipped", baseline, True, False, False, False))
        manual = copy.deepcopy(baseline)
        for name in audits:
            manual[name]["result"] = "success"
        scenarios.append(("manual quick gate with all audits", manual, True, False, False, True))
        scenarios.append(("qualification missing full suites", manual, True, True, False, False))
        qualified = copy.deepcopy(manual)
        for name in frontends | {"go_tests"}:
            qualified[name]["result"] = "success"
        scenarios.append(("complete qualification", qualified, True, True, False, True))
        for name in frontends | {"go_tests"}:
            state = copy.deepcopy(qualified)
            state[name]["result"] = "skipped"
            scenarios.append((f"qualification {name} skipped", state, True, True, False, False))
            state = copy.deepcopy(qualified)
            del state[name]
            scenarios.append((f"qualification {name} missing", state, True, True, False, False))
        scenarios.append(("requested history skipped", manual, True, False, True, False))
        historical = copy.deepcopy(manual)
        historical["historical_control"]["result"] = "success"
        scenarios.append(("requested history passed", historical, True, False, True, True))
        for label, jobs, manual_run, qualification, historical_run, success in scenarios:
            with self.subTest(label=label):
                env = {**os.environ, "NEEDS": json.dumps(jobs),
                       "MANUAL_RUN": str(manual_run).lower(), "PUBLIC_REPOSITORY": "true",
                       "QUALIFICATION": str(qualification).lower(),
                       "HISTORICAL_CONTROL": str(historical_run).lower(),
                       "NATIVE_ARM64_IMAGES": "false", "NATIVE_AMD64_IMAGES": "false"}
                result = subprocess.run([sys.executable, "-c", script], env=env, text=True, capture_output=True, check=False)
                self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)

    def test_native_arm64_qualification_uses_read_only_native_runner(self) -> None:
        """test_native_arm64_qualification_uses_read_only_native_runner guards isolation and selection."""
        job = self.ci["jobs"]["native_arm64_images"]
        self.assertEqual(job["runs-on"], "ubuntu-24.04-arm")
        self.assertEqual(job["needs"], "changes")
        self.assertEqual(job["if"], "github.event.repository.private == false && github.event_name == 'workflow_dispatch' && inputs.native_arm64_images")
        self.assertLessEqual(int(job["timeout-minutes"]), 120)
        self.assertEqual(job["steps"][0]["with"], {
            "ref": "${{ github.event.pull_request.head.sha || github.sha }}",
            "fetch-depth": "0", "persist-credentials": "false",
        })
        self.assertNotIn("permissions", job)
        self.assertNotIn("environment", job)
        self.assertNotIn("services", job)
        self.assertIn("scripts/qualify_native_arm64.py --evidence", commands(job))
        for forbidden in ("setup-qemu", "setup-buildx", "login-action", "build-push-action", "sudo", "--privileged", "docker push"):
            self.assertNotIn(forbidden, json.dumps(job))
        archive = job["steps"][-1]
        self.assertEqual(archive["if"], "always()")
        self.assertEqual(archive["with"]["path"], "${{ runner.temp }}/native-arm64-evidence")
        self.assertEqual(archive["with"]["retention-days"], "14")
        self.assertEqual(self.ci["on"]["workflow_dispatch"]["inputs"]["native_arm64_images"]["default"], "false")
        changes = self.ci["jobs"]["changes"]
        self.assertIn("python3 scripts/test_qualify_native_arm64.py", commands(changes))
        self.assertNotIn("media_runtime", changes["outputs"])
        filters = next(step["with"]["filters"] for step in changes["steps"] if step.get("id") == "filter")
        self.assertNotIn("media_runtime", yaml.load(filters, Loader=UniqueKeyLoader))

    def test_manual_native_documentation_identifies_candidate_and_receipt(self) -> None:
        """test_manual_native_documentation_identifies_candidate_and_receipt guards operator guidance."""
        doc = (ROOT / "docs/media-runtime-qualification.md").read_text()
        for architecture in ("ARM64", "AMD64"):
            section = doc.split(f"## Manual native {architecture} qualification", 1)[1].split("\n## ", 1)[0]
            self.assertIn("candidate branch", section)
            self.assertIn("`github.sha`", section)
            self.assertIn("`source-receipt.json`", section)
            self.assertIn("`head` matching the intended candidate commit", section)
            job = self.ci["jobs"][f"native_{architecture.lower()}_images"]
            self.assertEqual(job["steps"][0]["with"]["ref"],
                             "${{ github.event.pull_request.head.sha || github.sha }}")
            build = next(step for step in job["steps"] if "qualify_native_arm64.py" in step.get("run", ""))
            self.assertEqual(build["env"]["QUALIFICATION_HEAD"],
                             "${{ github.event.pull_request.head.sha || github.sha }}")
            self.assertIn('--expected-head "$QUALIFICATION_HEAD"', build["run"])
        self.assertNotIn("checks out the exact PR head", doc)

    def test_native_amd64_qualification_is_manual_and_isolated(self) -> None:
        """test_native_amd64_qualification_is_manual_and_isolated protects explicit native selection."""
        job = self.ci["jobs"]["native_amd64_images"]
        arm = self.ci["jobs"]["native_arm64_images"]
        self.assertEqual(self.ci["on"]["workflow_dispatch"]["inputs"]["native_amd64_images"],
                         {"description": "Qualify native AMD64 runtime images without publishing",
                          "type": "boolean", "default": "false"})
        self.assertEqual(job["if"], "github.event.repository.private == false && github.event_name == 'workflow_dispatch' && inputs.native_amd64_images")
        self.assertEqual(job["needs"], "changes")
        self.assertEqual(job["runs-on"], "ubuntu-24.04")
        self.assertEqual(job["timeout-minutes"], "120")
        self.assertEqual(job["steps"][0]["with"], arm["steps"][0]["with"])
        self.assertEqual([step["uses"] for step in job["steps"] if "uses" in step],
                         [step["uses"] for step in arm["steps"] if "uses" in step])
        for forbidden in ("permissions", "environment", "services", "strategy", "continue-on-error"):
            self.assertNotIn(forbidden, job)
        for step in job["steps"]:
            self.assertNotIn("continue-on-error", step)
        build = next(step for step in job["steps"] if "qualify_native_arm64.py" in step.get("run", ""))
        self.assertEqual(build["run"],
                         'python3 scripts/qualify_native_arm64.py --architecture amd64 --evidence '
                         '"$RUNNER_TEMP/native-amd64-evidence" --expected-head "$QUALIFICATION_HEAD" '
                         '--baseline-ref "refs/remotes/origin/${QUALIFICATION_BASE_REF#refs/heads/}" --baseline-target "$QUALIFICATION_BASE_TARGET"\n')
        self.assertEqual(build["env"], {
            "QUALIFICATION_HEAD": "${{ github.event.pull_request.head.sha || github.sha }}",
            "QUALIFICATION_BASE_REF": "${{ github.event.pull_request.base.ref || github.event.merge_group.base_ref || github.event.repository.default_branch }}",
            "QUALIFICATION_BASE_TARGET": "${{ github.event.pull_request.base.sha || github.event.merge_group.base_sha || format('refs/remotes/origin/{0}', github.event.repository.default_branch) }}",
            "GOMAXPROCS": "2", "GOFLAGS": "-p=2"})
        archive = job["steps"][-1]
        self.assertEqual(archive["if"], "always()")
        self.assertEqual(archive["with"], {
            "name": "native-amd64-images-${{ github.event.pull_request.head.sha || github.sha }}",
            "path": "${{ runner.temp }}/native-amd64-evidence",
            "if-no-files-found": "warn", "retention-days": "14"})
        self.assertNotEqual(archive["with"]["path"], arm["steps"][-1]["with"]["path"])
        self.assertNotEqual(archive["with"]["name"], arm["steps"][-1]["with"]["name"])
        for forbidden in ("setup-qemu", "setup-buildx", "login-action", "build-push-action",
                          "sudo", "--privileged", "docker push"):
            self.assertNotIn(forbidden, json.dumps(job))
        arm_build = next(step for step in arm["steps"] if "qualify_native_arm64.py" in step.get("run", ""))
        self.assertEqual(arm_build["run"],
                         'python3 scripts/qualify_native_arm64.py --evidence '
                         '"$RUNNER_TEMP/native-arm64-evidence" --expected-head "$QUALIFICATION_HEAD" '
                         '--baseline-ref "refs/remotes/origin/${QUALIFICATION_BASE_REF#refs/heads/}" --baseline-target "$QUALIFICATION_BASE_TARGET"\n')

    def test_native_baseline_comes_from_base_event_not_candidate_head(self) -> None:
        """test_native_baseline_comes_from_base_event_not_candidate_head binds immutable event provenance."""
        for name in ("native_arm64_images", "native_amd64_images"):
            step = next(step for step in self.ci["jobs"][name]["steps"]
                        if "qualify_native_arm64.py" in step.get("run", ""))
            env = step["env"]
            self.assertEqual(env["QUALIFICATION_BASE_REF"],
                             "${{ github.event.pull_request.base.ref || github.event.merge_group.base_ref || github.event.repository.default_branch }}")
            self.assertEqual(env["QUALIFICATION_BASE_TARGET"],
                             "${{ github.event.pull_request.base.sha || github.event.merge_group.base_sha || format('refs/remotes/origin/{0}', github.event.repository.default_branch) }}")
            self.assertNotIn("github.sha", env["QUALIFICATION_BASE_TARGET"])
            self.assertNotIn("github.event.before", env["QUALIFICATION_BASE_TARGET"])
            for branch, wanted in (("main", "refs/remotes/origin/main"),
                                   ("refs/heads/release/next", "refs/remotes/origin/release/next")):
                result = subprocess.check_output(["bash", "-c",
                    'printf "%s" "refs/remotes/origin/${QUALIFICATION_BASE_REF#refs/heads/}"'],
                    env={**os.environ, "QUALIFICATION_BASE_REF": branch}, text=True)
                self.assertEqual(result, wanted)

    def test_native_aggregate_rejects_incomplete_selected_results(self) -> None:
        """test_native_aggregate_rejects_incomplete_selected_results executes the shipped gate."""
        gate = self.ci["jobs"]["required"]
        step = gate["steps"][0]
        script = step["run"].split("<<'PY'\n", 1)[1].rsplit("\nPY", 1)[0]
        for architecture in ("arm64", "amd64"):
            name = f"native_{architecture}_images"
            variable = name.upper()
            self.assertIn(name, gate["needs"])
            self.assertEqual(step["env"][variable],
                             "${{ github.event_name == 'workflow_dispatch' && inputs." + name + " }}")
            for event in ("pull_request", "push", "merge_group", "workflow_dispatch"):
                for requested in (False, True):
                    for status in ("success", "failure", "cancelled", "skipped", "missing"):
                        with self.subTest(architecture=architecture, event=event,
                                          requested=requested, status=status):
                            jobs = {job: {"result": "success"} for job in gate["needs"]}
                            jobs["changes"]["outputs"] = {theme: "false" for theme in THEMES}
                            if status == "missing":
                                del jobs[name]
                            else:
                                jobs[name]["result"] = status
                            manual = event == "workflow_dispatch"
                            selected = manual and requested
                            env = {**os.environ, "NEEDS": json.dumps(jobs),
                                   "MANUAL_RUN": str(manual).lower(), "PUBLIC_REPOSITORY": "true", "QUALIFICATION": "false",
                                   "HISTORICAL_CONTROL": "false", "NATIVE_ARM64_IMAGES": "false",
                                   "NATIVE_AMD64_IMAGES": "false", variable: str(selected).lower()}
                            result = subprocess.run([sys.executable, "-c", script], env=env,
                                                    text=True, capture_output=True, check=False)
                            success = status == "success" or (not selected and status in {"skipped", "missing"})
                            self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)


    def test_private_native_requests_fail_explicitly(self) -> None:
        """test_private_native_requests_fail_explicitly checks eligibility without hiding selected skips."""
        gate = self.ci["jobs"]["required"]
        step = gate["steps"][0]
        self.assertEqual(step["env"]["PUBLIC_REPOSITORY"],
                         "${{ github.event.repository.private == false }}")
        script = step["run"].split("<<'PY'\n", 1)[1].rsplit("\nPY", 1)[0]
        for architecture in ("ARM64", "AMD64"):
            for public in (False, True):
                for manual in (False, True):
                    for requested in (False, True):
                        with self.subTest(architecture=architecture, public=public, manual=manual, requested=requested):
                            jobs = {name: {"result": "success"} for name in gate["needs"]}
                            jobs["changes"]["outputs"] = {theme: "false" for theme in THEMES}
                            jobs[f"native_{architecture.lower()}_images"]["result"] = "skipped"
                            env = {**os.environ, "NEEDS": json.dumps(jobs),
                                   "MANUAL_RUN": str(manual).lower(), "PUBLIC_REPOSITORY": str(public).lower(),
                                   "QUALIFICATION": "false", "HISTORICAL_CONTROL": "false",
                                   "NATIVE_ARM64_IMAGES": "false", "NATIVE_AMD64_IMAGES": "false",
                                   f"NATIVE_{architecture}_IMAGES": str(manual and requested).lower()}
                            result = subprocess.run([sys.executable, "-c", script], env=env,
                                                    text=True, capture_output=True, check=False)
                            self.assertEqual(result.returncode == 0, not (manual and requested))
                            if manual and requested and not public:
                                self.assertIn("Native image qualification requires a public repository", result.stderr)


if __name__ == "__main__":
    unittest.main(verbosity=2)
