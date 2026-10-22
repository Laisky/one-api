#!/usr/bin/env python3
"""Native orchestration contracts; these tests do not simulate runtime qualification."""
import argparse
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import types
import unittest
from unittest import mock
import qualify_native_arm64 as native


class NativeQualificationTests(unittest.TestCase):
    """NativeQualificationTests verify architecture, actual history and artifact isolation."""

    def historical(self, revision):
        """historical reads the actual immutable control recipe."""
        return subprocess.check_output(["git", "show", revision+":Dockerfile"],
            cwd=Path(__file__).resolve().parents[1], text=True)

    def test_native_architecture_and_compiler(self):
        """test_native_architecture_and_compiler rejects foreign hosts, daemons or compilers."""
        go = {"GOARCH": "arm64", "GOHOSTARCH": "arm64", "GOOS": "linux",
              "GOHOSTOS": "linux", "GOVERSION": "go1.27.3"}
        native.assert_native("aarch64", "arm64", go)
        for host, docker, compiler in (("x86_64", "arm64", go), ("arm64", "amd64", go),
                ("arm64", "arm64", dict(go, GOHOSTARCH="amd64")),
                ("arm64", "arm64", dict(go, GOVERSION="go1.26.0")),
                ("arm64", "arm64", dict(go, GOVERSION="go1.27.2"))):
            with self.subTest(host=host, docker=docker), self.assertRaises(RuntimeError):
                native.assert_native(host, docker, compiler)

    def test_actual_historical_controls(self):
        """test_actual_historical_controls verifies the real ownership change and strict scope."""
        self.assertEqual(native.ORIGINAL, "f4f26683404611c2313594e98e70ff5b36647dfd")
        self.assertEqual(native.OWNERSHIP, "ec72e68fc2c30363402e974d267674339159f0c2")
        self.assertEqual(native.PRODUCTION_BASELINE, "d316af5675f1363ec8d06ed3e92c15ed0d9fa0a4")
        original, ownership = self.historical(native.ORIGINAL), self.historical(native.OWNERSHIP)
        self.assertEqual(native.normalized_recipe(original), native.normalized_recipe(ownership))
        self.assertIn("COPY one-api /one-api", native.runtime_recipe(original))
        self.assertIn("COPY --chown=${ONEAPI_UID}:${ONEAPI_GID} one-api /one-api",
                      native.runtime_recipe(ownership))
        self.assertNotIn("--from=go-builder", native.runtime_recipe(original))
        changed = ownership.replace("curl libsqlite3-0", "curl unapproved-tool libsqlite3-0")
        self.assertNotEqual(native.normalized_recipe(original), native.normalized_recipe(changed))

    def test_actual_production_builder(self):
        """test_actual_production_builder catches altered build flags independently of Go sources."""
        current = (Path(__file__).resolve().parents[1]/"Dockerfile").read_text()
        baseline = self.historical("d316af5675f1363ec8d06ed3e92c15ed0d9fa0a4")
        self.assertEqual(native.builder_recipe(current), native.builder_recipe(baseline))
        self.assertNotEqual(native.builder_recipe(current.replace('-ldflags "-s -w"', '-ldflags "-s"')),
                            native.builder_recipe(baseline))


    def production_fixture(self, root):
        """production_fixture creates a tiny real Git history using the approved builder recipe."""
        def git(*args):
            """git executes only this test-owned repository's Git commands."""
            return subprocess.check_output(["git", *args], cwd=root, text=True, stderr=subprocess.STDOUT).strip()
        git("init", "-q")
        (root/"Dockerfile").write_text(self.historical(native.PRODUCTION_BASELINE))
        (root/"go.mod").write_text("module fixture\n\ngo 1.27.3\n")
        (root/"main.go").write_text("package main\n")
        (root/"web").mkdir()
        (root/"web"/"安全.txt").write_text("original public fixture")
        (root/".dockerignore").write_text(".git\n")
        (root/"model").mkdir()
        (root/"model"/"dashboard_aggregate.sql").write_text("SELECT 1;\n")
        (root/"scripts").mkdir()
        (root/"scripts"/"docker-entrypoint.sh").write_text("#!/bin/sh\n")
        def commit():
            """commit records synthetic public fixture changes with an isolated author identity."""
            git("add", ".")
            git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                "commit", "-qm", "public synthetic fixture")
            return git("rev-parse", "HEAD")
        return git, commit, commit()

    def test_source_guard_accepts_approved_baseline_and_rejects_production_drift(self):
        """test_source_guard_accepts_approved_baseline_and_rejects_production_drift exercises real Git paths."""
        for change in ("test-only", "go", "web-unicode", "go-rename", "entrypoint", "module", "builder",
                       "embedded-sql", "dockerignore", "wrong-head", "dirty", "nonancestor", "absent-baseline"):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)/"repository"
                root.mkdir()
                git, commit, baseline = self.production_fixture(root)
                if change == "test-only":
                    (root/"main_test.go").write_text("package main\n")
                elif change == "go":
                    (root/"main.go").write_text("package changed\n")
                elif change == "web-unicode":
                    (root/"web"/"安全.txt").write_text("changed public fixture")
                elif change == "go-rename":
                    (root/"main.go").rename(root/"main_test.go")
                elif change == "entrypoint":
                    (root/"scripts"/"docker-entrypoint.sh").write_text("#!/bin/false\n")
                elif change == "module":
                    (root/"go.mod").write_text("module fixture\n\ngo 1.27.2\n")
                elif change == "embedded-sql":
                    (root/"model"/"dashboard_aggregate.sql").write_text("SELECT 2;\n")
                elif change == "dockerignore":
                    (root/".dockerignore").write_text("web/build\n")
                elif change in ("wrong-head", "dirty", "nonancestor", "absent-baseline"):
                    (root/"qualification_test.go").write_text("package main\n")
                else:
                    dockerfile = root/"Dockerfile"
                    dockerfile.write_text(dockerfile.read_text().replace('-ldflags "-s -w"', '-ldflags "-s"'))
                head = commit()
                qualification = native.Qualification(argparse.Namespace(
                    evidence=Path(temporary)/"evidence", timeout=300,
                    expected_head="0"*40 if change == "wrong-head" else head))
                qualification.repo = root
                if change == "dirty":
                    (root/"main.go").write_text("package dirty\n")
                if change == "nonancestor":
                    baseline = git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                                   "commit-tree", head+"^{tree}", "-m", "unrelated public root")
                if change == "absent-baseline":
                    baseline = "f"*40
                try:
                    with mock.patch.object(native, "PRODUCTION_BASELINE", baseline):
                        if change == "test-only":
                            qualification.source()
                            receipt = json.loads((qualification.evidence/"source-receipt.json").read_text())
                            self.assertEqual(receipt["production_baseline"], baseline)
                            self.assertEqual(receipt["production_changes_since_baseline"], [])
                        else:
                            if change in ("nonancestor", "absent-baseline"):
                                with self.assertRaises(RuntimeError):
                                    qualification.source()
                            else:
                                wanted = {"wrong-head": "checkout differs", "dirty": "clean exact-head"}
                                with self.assertRaisesRegex(RuntimeError, wanted.get(
                                        change, "differs from approved baseline")):
                                    qualification.source()
                finally:
                    shutil.rmtree(qualification.work)


    def test_application_compiler_is_bound_to_actual_extracted_metadata(self):
        """test_application_compiler_is_bound_to_actual_extracted_metadata rejects stale or missing build data."""
        native.assert_application_compiler("/fixture/one-api: go1.27.3\n\tpath github.com/Laisky/one-api\n")
        for metadata in ("/fixture/one-api: go1.27.2\n", "not a Go executable\n",
                         "/fixture/one-api: go1.27.30\n", "stale\n/fixture/one-api: go1.27.3\n"):
            with self.subTest(metadata=metadata), self.assertRaises(RuntimeError):
                native.assert_application_compiler(metadata)

    def test_builder_inputs_select_fixed_compiler_without_running_docker(self):
        """test_builder_inputs_select_fixed_compiler_without_running_docker records actual pull arguments."""
        calls = []
        def run(argv, _name, *_args):
            """run records image-selection commands and returns one valid native platform descriptor."""
            calls.append(argv)
            return json.dumps({"manifests": [{"platform": {"os": "linux", "architecture": "arm64"},
                                              "digest": "sha256:"+"a"*64}]})
        fake = types.SimpleNamespace(run=run, identity=lambda reference, _name: {
            "Id": "sha256:"+"b"*64, "Size": 1, "RepoDigests": [reference.split(":")[0]+"@sha256:"+"c"*64]},
            summary={}, flush=lambda: None)
        native.Qualification.inputs(fake)
        pulls = [argv[-1] for argv in calls if argv[:2] == ["docker", "pull"]]
        self.assertIn("golang:1.27.3-bookworm", pulls)
        self.assertNotIn("golang:1.27.2-bookworm", pulls)

    def test_actual_collector_manifest(self):
        """test_actual_collector_manifest consumes the real collector schema and rejects path escapes."""
        spec = importlib.util.spec_from_file_location("native_test_collector",
                                                      Path(__file__).with_name("collect_media_runtime.py"))
        collector = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(collector)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root/"source"
            source.write_bytes(b"public synthetic fixture")
            collector.ROOT = root/"output"
            collector.copy_file(source, collector.ROOT/"opt/oneapi-media/bin/ffmpeg", role="program")
            manifest = {"files": collector.RECORDS}
            records = native.runtime_records(manifest)
            self.assertEqual(records[0]["destination"], "/opt/oneapi-media/bin/ffmpeg")
            self.assertEqual(manifest["files"][0]["destination"], "opt/oneapi-media/bin/ffmpeg")
        for destination in ("/etc/passwd", "../escape", "opt/../escape", "a;echo", "", "etc/passwd", "opt//oneapi-media/a", "./opt/oneapi-media/a"):
            with self.subTest(destination=destination), self.assertRaises(RuntimeError):
                native.runtime_records({"files": [{"destination": destination}]})

    def test_independent_elf_machine(self):
        """test_independent_elf_machine distinguishes ARM64 from AMD64 bytes and rejects non-ELF."""
        header = bytearray(20)
        header[:6] = b"\x7fELF\x02\x01"
        header[18:20] = (183).to_bytes(2, "little")
        self.assertEqual(native.elf_machine(header), 183)
        header[18:20] = (62).to_bytes(2, "little")
        self.assertEqual(native.elf_machine(header), 62)
        with self.assertRaises(ValueError):
            native.elf_machine(b"not ELF")

    def test_evidence_excludes_runtime_payloads(self):
        """test_evidence_excludes_runtime_payloads checks the upload allowlist with real files."""
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            qualification = native.Qualification(argparse.Namespace(evidence=root/"evidence", timeout=300))
            try:
                suite = root/"suite"
                for name in ("run-1/summary.json", "run-1/tts-artifacts/mp3.json", "run-1/helper.log",
                             "corpus/manifest.json", "test-binaries/receipt.json",
                             "run-1/tts-artifacts/encoded.mp3", "server.key", "helper.test"):
                    path = suite/name
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_text("{}")
                qualification.copy_reports(suite)
                copied = qualification.evidence/"media-reports"
                self.assertEqual({str(path.relative_to(copied)) for path in copied.rglob("*") if path.is_file()},
                                 {"run-1/summary.json", "run-1/tts-artifacts/mp3.json", "run-1/helper.log",
                                  "test-binaries/receipt.json"})
            finally:
                shutil.rmtree(qualification.work)

    def test_owned_process_group_deadline(self):
        """test_owned_process_group_deadline exercises real supervision using a harmless local child."""
        with tempfile.TemporaryDirectory() as temporary:
            qualification = native.Qualification(argparse.Namespace(evidence=Path(temporary)/"evidence", timeout=300))
            try:
                commands = ["import time; time.sleep(60)",
                    "import subprocess,sys; subprocess.Popen([sys.executable,'-c','import time; time.sleep(60)'])",
                    "import subprocess,sys; subprocess.Popen([sys.executable,'-c','import signal,time; signal.signal(signal.SIGTERM,signal.SIG_IGN); time.sleep(60)'])"]
                for command in commands:
                    with self.subTest(command=command), mock.patch.object(qualification, "guard"):
                        with self.assertRaisesRegex(RuntimeError, "command deadline"):
                            qualification.run([sys.executable, "-c", command], "owned-timeout", timeout=0.05)
                receipt = json.loads((qualification.evidence/"summary.json").read_text())
                self.assertEqual(receipt["commands"][-1]["state"], "failed")
                self.assertLess(receipt["commands"][-1]["seconds"], 15)
            finally:
                shutil.rmtree(qualification.work)


if __name__ == "__main__":
    unittest.main(verbosity=2)
