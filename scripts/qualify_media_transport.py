#!/usr/bin/env python3
"""Qualify media hostname/TLS transport using disposable, network-isolated fixtures."""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile
import uuid
import wave


def run(*args, **kwargs):
    """run executes one bounded fixture command without shell interpolation."""
    return subprocess.run(args, check=True, timeout=120, capture_output=True,
                          text=True, **kwargs)


def qualify(image):
    """qualify tests a trusted image's private media TLS and hostname behavior."""
    with tempfile.TemporaryDirectory(prefix="oneapi-media-transport-") as parent:
        fixture = Path(parent)
        fixture.chmod(0o755)
        scratch = fixture / "scratch"
        scratch.mkdir(mode=0o777)
        scratch.chmod(0o777)
        for name in ("server", "untrusted"):
            run("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
                "-keyout", str(fixture / f"{name}.key"), "-out", str(fixture / f"{name}.pem"),
                "-days", "1", "-subj", "/CN=media.fixture.test",
                "-addext", "subjectAltName=DNS:media.fixture.test")
            # These are newly generated disposable fixture keys, never real credentials.
            (fixture / f"{name}.key").chmod(0o644)
        with wave.open(str(fixture / "sample.wav"), "wb") as audio:
            audio.setnchannels(1)
            audio.setsampwidth(2)
            audio.setframerate(24000)
            audio.writeframes(b"\0\0" * 48000)
        command = r'''set -eu
openssl s_server -accept 8443 -key /fixture/server.key -cert /fixture/server.pem \
  -WWW -quiet > /scratch/server.log 2>&1 &
server=$!
trap 'kill "$server" 2>/dev/null || true; wait "$server" 2>/dev/null || true' EXIT
count=0
until curl --noproxy '*' --silent --fail --cacert /fixture/server.pem \
  https://media.fixture.test:8443/sample.wav -o /dev/null; do
  count=$((count + 1))
  test "$count" -lt 50 || exit 90
  sleep 0.1
done
ffprobe -v error -rw_timeout 5000000 -tls_verify 1 -ca_file /fixture/server.pem \
  -show_entries format=format_name,duration:stream=codec_name,sample_rate,channels -of json \
  https://media.fixture.test:8443/sample.wav
if ffprobe -v error -rw_timeout 5000000 -tls_verify 1 -ca_file /fixture/untrusted.pem \
  -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 \
  https://media.fixture.test:8443/sample.wav > /scratch/rejected.log 2>&1; then
  exit 91
fi
test -s /scratch/rejected.log
'''
        name = "oneapi-media-transport-" + uuid.uuid4().hex[:12]
        try:
            result = run("docker", "run", "--rm", "--name", name,
                     "--network", "none", "--read-only",
                     "--user", "10001:10001", "--memory", "256m", "--cpus", "1",
                     "--pids-limit", "64", "--add-host", "media.fixture.test:127.0.0.1",
                     "--mount", f"type=bind,src={fixture},dst=/fixture,readonly",
                     "--mount", f"type=bind,src={scratch},dst=/scratch",
                     "--workdir", "/fixture", "--entrypoint", "/bin/sh", image,
                     "-c", command)
        finally:
            subprocess.run(["docker", "rm", "-f", name], timeout=30,
                           capture_output=True, text=True, check=False)
        metadata = json.loads(result.stdout)
        if metadata["format"]["format_name"] != "wav" or metadata["streams"] != [
                {"codec_name": "pcm_s16le", "sample_rate": "24000", "channels": 1}]:
            raise AssertionError(f"unexpected HTTPS probe metadata: {metadata}")
        return {"image": image, "probe": metadata, "tls_verified": True,
                "untrusted_ca_rejected": True, "hostname_resolved": "media.fixture.test",
                "hostname_source": "isolated container hosts fixture, no external DNS",
                "runtime_uid": 10001, "external_network": False}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("images", nargs="+")
    args = parser.parse_args()
    results = [qualify(image) for image in args.images]
    if any(item["probe"] != results[0]["probe"] for item in results[1:]):
        raise AssertionError("baseline and candidate HTTPS probe results differ")
    print(json.dumps(results, indent=2))
