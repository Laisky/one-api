#!/usr/bin/env python3
"""Exercise a built image's real entrypoint, SQLite storage and runtime UID contract.

Run explicitly with ONEAPI_RUNTIME_TEST_IMAGE set to a local qualification tag.
Every container uses synthetic temporary data, no published ports and no network.
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest
import uuid

IMAGE = os.environ.get("ONEAPI_RUNTIME_TEST_IMAGE")


def docker(*args, check=True):
    """docker runs one bounded Docker operation and returns captured text output."""
    return subprocess.run(["docker", *args], capture_output=True, text=True,
                          timeout=120, check=check)


@unittest.skipUnless(IMAGE, "set ONEAPI_RUNTIME_TEST_IMAGE for image integration")
class RuntimeImageTests(unittest.TestCase):
    """RuntimeImageTests verify the image contract using its actual executable."""

    def setUp(self):
        """setUp creates a unique synthetic directory and container identity."""
        self.temp = tempfile.TemporaryDirectory(prefix="oneapi-runtime-test-")
        self.data = Path(self.temp.name) / "data"
        self.data.mkdir(mode=0o755)
        self.name = "oneapi-runtime-test-" + uuid.uuid4().hex[:12]

    def tearDown(self):
        """tearDown removes only this test's container and synthetic owned files."""
        docker("rm", "-f", self.name, check=False)
        docker("run", "--rm", "--network", "none", "--cap-drop", "ALL",
               "--cap-add", "CHOWN", "--entrypoint", "/bin/sh",
               "--mount", f"type=bind,src={self.data},dst=/fixture", IMAGE,
               "-c", f"chown -R {os.getuid()}:{os.getgid()} /fixture", check=False)
        self.temp.cleanup()

    def start(self, *, user=None, theme="modern"):
        """start launches the real entrypoint and waits for its HTTP health response."""
        args = ["run", "-d", "--name", self.name, "--network", "none",
                "--memory", "512m", "--cpus", "1", "--pids-limit", "128",
                "--mount", f"type=bind,src={self.data},dst=/data",
                "-e", "SQLITE_PATH=/data/sqlite/qualification.db",
                "-e", f"THEME={theme}", "-e", "TZ=Etc/UTC"]
        if user:
            args.extend(["--user", user])
        docker(*args, IMAGE, "--log-dir", "/data/custom/logs")
        for _ in range(90):
            result = docker("exec", self.name, "curl", "-fsS",
                            "http://127.0.0.1:3000/api/status", check=False)
            if result.returncode == 0:
                self.assertTrue(json.loads(result.stdout)["success"])
                return
            state = json.loads(docker("inspect", self.name).stdout)[0]["State"]
            self.assertTrue(state["Running"], "application exited before healthy")
            time.sleep(0.5)
        self.fail("application did not become healthy")

    def assert_runtime(self, uid="10001", gid="10001"):
        """assert_runtime checks privilege dropping, storage ownership and served UI."""
        result = docker("exec", self.name, "/bin/sh", "-c",
                        "grep '^Uid:' /proc/1/status; grep '^Gid:' /proc/1/status")
        lines = result.stdout.splitlines()
        self.assertEqual(lines[0].split()[1:], [uid] * 4)
        self.assertEqual(lines[1].split()[1:], [gid] * 4)
        for relative in ("sqlite/qualification.db", "custom/logs"):
            value = self.data / relative
            self.assertTrue(value.exists(), relative)
            self.assertEqual(value.stat().st_uid, int(uid))
            self.assertEqual(value.stat().st_gid, int(gid))
        page = docker("exec", self.name, "curl", "-fsS", "http://127.0.0.1:3000/")
        self.assertIn("<html", page.stdout.lower())

    def test_root_entrypoint_initializes_mounted_data_and_drops_privileges(self):
        """test_root_entrypoint checks ownership repair and new SQLite initialization."""
        self.start()
        self.assert_runtime()
        before = (self.data / "sqlite/qualification.db").stat().st_size
        docker("stop", "--time", "30", self.name)
        docker("start", self.name)
        for _ in range(60):
            if docker("exec", self.name, "curl", "-fsS",
                      "http://127.0.0.1:3000/api/status", check=False).returncode == 0:
                break
            time.sleep(0.5)
        self.assert_runtime()
        self.assertGreaterEqual((self.data / "sqlite/qualification.db").stat().st_size, before)

    def test_preowned_bind_mount_runs_as_nonroot(self):
        """test_preowned_bind_mount checks an explicit non-root runtime user."""
        docker("run", "--rm", "--network", "none", "--entrypoint", "/bin/sh",
               "--mount", f"type=bind,src={self.data},dst=/fixture", IMAGE,
               "-c", "chown 10001:10001 /fixture")
        self.start(user="10001:10001")
        self.assert_runtime()

    def test_binary_permissions_and_native_media_tools(self):
        """test_binary_permissions checks executable ownership and retained CLI tools."""
        output = docker("run", "--rm", "--network", "none", "--read-only",
                        "--user", "10001:10001", "--entrypoint", "/bin/sh", IMAGE,
                        "-c", "stat -c '%u:%g %a' /one-api; "
                        "test -x /usr/local/bin/docker-entrypoint.sh && "
                        "ffmpeg -version >/dev/null && ffprobe -version >/dev/null && "
                        "test -s /etc/ssl/certs/ca-certificates.crt && "
                        "TZ=Etc/UTC date +%Z")
        self.assertEqual(output.stdout.splitlines()[0], "10001:10001 755")
        self.assertEqual(output.stdout.splitlines()[-1], "UTC")


if __name__ == "__main__":
    unittest.main(verbosity=2)

