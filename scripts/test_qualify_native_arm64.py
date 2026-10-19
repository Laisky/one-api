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
              "GOHOSTOS": "linux", "GOVERSION": "go1.27.2"}
        native.assert_native("aarch64", "arm64", go)
        for host, docker, compiler in (("x86_64", "arm64", go), ("arm64", "amd64", go),
                ("arm64", "arm64", dict(go, GOHOSTARCH="amd64")),
                ("arm64", "arm64", dict(go, GOVERSION="go1.26.0"))):
            with self.subTest(host=host, docker=docker), self.assertRaises(RuntimeError):
                native.assert_native(host, docker, compiler)

    def test_actual_historical_controls(self):
        """test_actual_historical_controls verifies the real ownership change and strict scope."""
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
        baseline = self.historical(native.OWNERSHIP)
        self.assertEqual(native.builder_recipe(current), native.builder_recipe(baseline))
        self.assertNotEqual(native.builder_recipe(current.replace('-ldflags "-s -w"', '-ldflags "-s"')),
                            native.builder_recipe(baseline))

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
                with mock.patch.object(qualification, "guard"):
                    with self.assertRaisesRegex(RuntimeError, "command deadline"):
                        qualification.run([sys.executable, "-c", "import time; time.sleep(60)"],
                                          "owned-timeout", timeout=0.05)
                receipt = json.loads((qualification.evidence/"summary.json").read_text())
                self.assertEqual(receipt["commands"][-1]["state"], "failed")
                self.assertLess(receipt["commands"][-1]["seconds"], 15)
            finally:
                shutil.rmtree(qualification.work)


if __name__ == "__main__":
    unittest.main(verbosity=2)
