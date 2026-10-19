#!/usr/bin/env python3
"""Qualify real media helpers and image utilities using a synthetic offline corpus.

Only explicitly supplied local images run. Entrypoints are always overridden,
containers have no network, and all mounted content is generated test material.
"""
import argparse
from decimal import Decimal, InvalidOperation, ROUND_CEILING
import hashlib
import json
import math
import os
from pathlib import Path
import re
import struct
import subprocess
import sys
import tempfile
import time
import uuid
import wave

PACKAGES = {
    "helper": ("./common/helper", "^TestMediaRuntime"),
    "tts": ("./relay/adaptor/gemini/tts",
            "^TestMediaRuntime|^TestSpeechEncoderFormats$|^TestWAVChunkValidation$|"
            "^TestSpeechEncodingFailureKeepsReceiptForCredit$|"
            "^TestSpeechCancellationAndWriteFailureDoNotRequestCredit$"),
    "controller": ("./relay/controller",
                   "^TestMediaRuntime|^TestProtocolAuditTranscriptionHTTP$|"
                   "^TestAudioTariffArithmetic$|^TestGeminiSpeechReviewSettlementHTTP$"),
}


def command(args, *, timeout=120, check=True, binary=False):
    """command executes one bounded local process without interpreting shell text."""
    result = subprocess.run(args, capture_output=True, text=not binary, timeout=timeout)
    if check and result.returncode:
        detail = result.stderr.decode(errors="replace") if binary else result.stderr
        raise RuntimeError(f"command failed ({result.returncode}): {args[0]}\n{detail[-6000:]}")
    return result


def save_json(path, value):
    """save_json records structured evidence with deterministic formatting."""
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n")


def sha256(path):
    """sha256 identifies one local synthetic fixture or compiled test binary."""
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def identity(image):
    """identity resolves a local image without fetching tags or inspecting its environment."""
    fmt = '{"id":{{json .Id}},"os":{{json .Os}},"architecture":{{json .Architecture}}}'
    return json.loads(command(["docker", "image", "inspect", "--format", fmt, image]).stdout)


def container(image, mounts, executable, arguments, *, timeout=120, binary=False):
    """container runs one isolated utility and always removes only its unique container."""
    name = "oneapi-media-" + uuid.uuid4().hex[:16]
    scratch = tempfile.TemporaryDirectory(prefix="oneapi-media-scratch-")
    args = ["docker", "run", "--rm", "--pull", "never", "--name", name,
            "--network", "none", "--read-only", "--cap-drop", "ALL",
            "--security-opt", "no-new-privileges", "--memory", "768m",
            "--cpus", "1", "--pids-limit", "128",
            "--user", f"{os.getuid()}:{os.getgid()}",
            "--mount", f"type=bind,src={scratch.name},dst=/tmp",
            "-e", "TMPDIR=/tmp",
            "-e", "TZ=Etc/UTC", "-e", "GOMAXPROCS=2"]
    for source, target, readonly in mounts:
        mode = ",readonly" if readonly else ""
        args += ["--mount", f"type=bind,src={source.resolve()},dst={target}{mode}"]
    args += ["--entrypoint", executable, image, *arguments]
    try:
        return command(args, timeout=timeout, check=False, binary=binary)
    finally:
        try:
            command(["docker", "rm", "-f", name], timeout=20, check=False)
        finally:
            scratch.cleanup()


def tool(image, corpus, program, args, *, binary=False, writable=False):
    """tool invokes the actual PATH-resolved media utility with an overridden entrypoint."""
    return container(image, [(corpus, "/fixtures", not writable)], "/bin/sh",
                     ['-c', 'exec "$@"', "media-qualification", program, *args],
                     binary=binary)


def generate_corpus(image, corpus):
    """generate_corpus creates finite deterministic fixtures with the baseline encoder."""
    corpus.mkdir()
    with wave.open(str(corpus / "tone.wav"), "wb") as stream:
        stream.setparams((1, 2, 24000, 0, "NONE", "not compressed"))
        stream.writeframes(b"".join(struct.pack("<h", int(10000 * math.sin(
            2 * math.pi * 440 * index / 24000))) for index in range(48000)))
    original = (corpus / "tone.wav").read_bytes()
    extra = original[:36] + b"JUNK\x03\0\0\0abc\0" + original[36:]
    extra = extra[:4] + struct.pack("<I", len(extra)-8) + extra[8:]
    (corpus / "wav-extra-chunks.wav").write_bytes(extra)
    (corpus / "wav-truncated.wav").write_bytes(original[:len(original)//2])
    (corpus / "corrupt.audio").write_bytes(b"synthetic invalid audio payload\n" * 17)
    (corpus / "empty.audio").write_bytes(b"")
    conversions = [
        ("mp3-cbr-xing.mp3", ["-c:a", "libmp3lame", "-b:a", "96k"]),
        ("mp3-cbr-no-xing.mp3", ["-c:a", "libmp3lame", "-b:a", "96k", "-write_xing", "0"]),
        ("mp3-vbr-xing.mp3", ["-c:a", "libmp3lame", "-q:a", "4"]),
        ("mp3-vbr-no-xing.mp3", ["-c:a", "libmp3lame", "-q:a", "4", "-write_xing", "0"]),
        ("audio.m4a", ["-c:a", "aac"]),
        ("audio.aac", ["-c:a", "aac", "-f", "adts"]),
        ("audio.flac", ["-c:a", "flac"]),
        ("audio-opus.ogg", ["-c:a", "libopus"]),
        ("audio-vorbis.ogg", ["-c:a", "libvorbis"]),
        ("audio.webm", ["-c:a", "libopus"]),
        ("unicode.flac", ["-c:a", "flac", "-metadata", "title=Synthetic caf\u00e9 \U0001f3b5"]),
        ("edit-list.mp4", ["-c:a", "aac", "-use_editlist", "1"]),
    ]
    for name, options in conversions:
        args = ["-hide_banner", "-v", "error", "-nostdin", "-y",
                "-threads", "1", "-i", "/fixtures/tone.wav",
                *options, "-threads", "1", "/fixtures/" + name]
        result = tool(image, corpus, "ffmpeg", args, writable=True)
        if result.returncode:
            raise RuntimeError(f"fixture codec unavailable for {name}: {result.stderr}")
    args = ["-hide_banner", "-v", "error", "-nostdin", "-y", "-threads", "1",
            "-f", "lavfi", "-i", "color=c=blue:s=16x16:r=10:d=3",
            "-i", "/fixtures/tone.wav", "-map", "0:v:0", "-map", "1:a:0",
            "-c:v", "mpeg4", "-c:a", "aac", "-threads", "1",
            "/fixtures/multistream.mp4"]
    result = tool(image, corpus, "ffmpeg", args, writable=True)
    if result.returncode:
        raise RuntimeError("multistream fixture generation failed: " + result.stderr)
    result = tool(image, corpus, "ffmpeg", [
        "-hide_banner", "-v", "error", "-nostdin", "-threads", "1",
        "-i", "/fixtures/tone.wav", "-c:a", "libopus", "-live", "1",
        "-f", "webm", "pipe:1"], binary=True)
    if result.returncode:
        raise RuntimeError("unknown-duration fixture generation failed")
    (corpus / "unknown-duration.webm").write_bytes(result.stdout)
    (corpus / "extensionless").write_bytes((corpus / "audio.m4a").read_bytes())
    (corpus / "mislabeled.mp3").write_bytes((corpus / "audio.m4a").read_bytes())
    (corpus / "mp3-truncated.mp3").write_bytes((corpus / "mp3-vbr-xing.mp3").read_bytes()[:400])
    malformed = bytearray(original)
    malformed[40:44] = struct.pack("<I", 0x7fffffff)
    (corpus / "wav-inconsistent-length.wav").write_bytes(malformed)
    edit = (corpus / "edit-list.mp4").read_bytes()
    if b"elst" not in edit:
        raise RuntimeError("edit-list fixture lacks the expected MP4 edit box")


def normalize_error(value):
    """normalize_error removes ASLR addresses while preserving diagnostic content."""
    return re.sub(r"0x[0-9a-fA-F]+", "0xADDRESS", value)


def observe(image, corpus, name):
    """observe captures real probe classification, metadata and decoded PCM identity."""
    path = "/fixtures/" + name
    duration = tool(image, corpus, "ffprobe", [
        "-v", "error", "-show_entries", "format=duration", "-of",
        "default=noprint_wrappers=1:nokey=1", path])
    metadata = tool(image, corpus, "ffprobe", [
        "-v", "error", "-show_entries",
        "format=format_name,duration:format_tags=title:"
        "stream=index,codec_name,codec_type,sample_rate,channels,duration",
        "-of", "json", path])
    decode = tool(image, corpus, "ffmpeg", [
        "-hide_banner", "-v", "error", "-nostdin", "-threads", "1",
        "-i", path, "-map", "0:a:0", "-ac", "1", "-ar", "24000",
        "-threads", "1", "-f", "s16le", "pipe:1"], binary=True)
    return {
        "duration_code": duration.returncode,
        "duration_stdout": duration.stdout,
        "duration_stderr": normalize_error(duration.stderr),
        "metadata_code": metadata.returncode,
        "metadata_stdout": metadata.stdout,
        "metadata_stderr": normalize_error(metadata.stderr),
        "decode_code": decode.returncode,
        "decode_bytes": len(decode.stdout),
        "decode_sha256": hashlib.sha256(decode.stdout).hexdigest(),
        "decode_stderr": normalize_error(decode.stderr.decode(errors="replace")),
    }


def manifest_case(name, observation, digest):
    """manifest_case derives the existing helper contract and an independent decimal quota."""
    duration = observation["duration_stdout"].strip()
    success = False
    try:
        parsed = Decimal(duration)
        success = (observation["duration_code"] == 0 and
                   not observation["duration_stderr"] and
                   parsed.is_finite() and parsed > 0)
    except InvalidOperation:
        parsed = Decimal(0)
    quota = int((parsed * 500).to_integral_value(rounding=ROUND_CEILING)) if success else 0
    return {"name": name, "path": name, "sha256": digest, "success": success,
            "duration": duration if success else "", "quota": quota}


def compile_tests(repo, destination):
    """compile_tests builds the real package test executables with bounded Go parallelism."""
    destination.mkdir(exist_ok=True)
    receipt = {}
    for name, (package, _) in PACKAGES.items():
        path = destination / (name + ".test")
        start = time.monotonic()
        result = command(["env", "GOMAXPROCS=2", "GOFLAGS=-p=2", "go",
                          "test", "-c", "-o", str(path), package],
                         timeout=1800, check=False)
        (destination / (name + "-build.log")).write_text(result.stdout + result.stderr)
        if result.returncode:
            raise RuntimeError(f"{name} test compilation failed; see its build log")
        receipt[name] = {"sha256": sha256(path), "seconds": time.monotonic()-start}
    save_json(destination / "receipt.json", receipt)


def run_go_suite(image, corpus, binaries, cache, output):
    """run_go_suite executes strict real helper, billing and codec tests inside one image."""
    output.mkdir()
    results = {}
    for name, (_, pattern) in PACKAGES.items():
        artifacts = output / (name + "-artifacts")
        artifacts.mkdir()
        args = ["-c", 'export ONEAPI_MEDIA_RUNTIME=1 ONEAPI_MEDIA_CORPUS_DIR=/fixtures '
                'ONEAPI_MEDIA_OUTPUT_DIR=/artifacts TIKTOKEN_CACHE_DIR=/tokenizer; '
                'exec "$@"', "media-go-test", "/tests/" + name + ".test",
                "-test.v", "-test.count=1", "-test.timeout=8m", "-test.run", pattern]
        result = container(image, [(corpus, "/fixtures", True),
            (binaries, "/tests", True), (cache, "/tokenizer", True),
            (artifacts, "/artifacts", False)], "/bin/sh", args, timeout=540)
        log = result.stdout + result.stderr
        (output / (name + ".log")).write_text(log)
        if result.returncode or "--- SKIP:" in log:
            raise RuntimeError(f"{name} actual-image Go tests failed or skipped; see {output/name}.log")
        if "--- PASS:" not in log:
            raise RuntimeError(f"{name} selected no tests")
        if name == "tts" and len(list(artifacts.glob("*.json"))) != 18:
            raise RuntimeError("speech matrix did not emit all 18 decoded-audio reports")
        results[name] = {"returncode": result.returncode,
                         "test_pass_lines": log.count("--- PASS:"),
                         "artifacts": len(list(artifacts.iterdir()))}
    return results


def compare_observations(baseline, candidate):
    """compare_observations rejects changed probe results, decoded audio, or diagnostics."""
    differences = {}
    for name in baseline:
        changed = {key: {"baseline": baseline[name][key], "candidate": candidate[name][key]}
                   for key in baseline[name] if baseline[name][key] != candidate[name][key]}
        if changed:
            differences[name] = changed
    return differences


def qualify(args):
    """qualify creates resumable evidence and compares actual baseline and candidate images."""
    repo = Path(__file__).resolve().parents[1]
    evidence = args.evidence.resolve()
    if evidence.exists() and not args.reuse:
        raise ValueError("evidence directory exists; use --reuse only for this suite's evidence")
    evidence.mkdir(parents=True, exist_ok=True)
    baseline_id = identity(args.baseline)
    candidate_id = identity(args.candidate) if args.candidate else None
    if candidate_id and baseline_id["architecture"] != candidate_id["architecture"]:
        raise ValueError("baseline and candidate architectures differ")
    if candidate_id and baseline_id["id"] == candidate_id["id"]:
        raise ValueError("candidate resolves to the baseline image")
    cache = args.tokenizer_cache.resolve()
    if not cache.is_dir():
        raise ValueError("provide the pre-populated public tokenizer cache")
    corpus, binaries = evidence/"corpus", evidence/"test-binaries"
    if not args.reuse:
        generate_corpus(args.baseline, corpus)
        baseline = {path.name: observe(args.baseline, corpus, path.name)
                    for path in sorted(corpus.iterdir())}
        cases = [manifest_case(name, obs, sha256(corpus/name))
                 for name, obs in baseline.items()]
        unknown = next(case for case in cases if case["name"] == "unknown-duration.webm")
        if unknown["success"]:
            raise RuntimeError("streaming WebM unexpectedly supplied a duration; unknown fixture invalid")
        save_json(corpus/"manifest.json", {"cases": cases})
        save_json(evidence/"baseline-observations.json", baseline)
        save_json(evidence/"baseline-identity.json", baseline_id)
        compile_tests(repo, binaries)
    else:
        if json.loads((evidence/"baseline-identity.json").read_text()) != baseline_id:
            raise ValueError("reuse baseline image identity changed")
        manifest = json.loads((corpus/"manifest.json").read_text())
        for case in manifest["cases"]:
            if sha256(corpus/case["path"]) != case["sha256"]:
                raise ValueError("reused corpus was modified")
        baseline = json.loads((evidence/"baseline-observations.json").read_text())
        receipts = json.loads((binaries/"receipt.json").read_text())
        for name, receipt in receipts.items():
            if sha256(binaries/(name+".test")) != receipt["sha256"]:
                raise ValueError("compiled test binary changed")
    if args.rebuild_tests:
        compile_tests(repo, binaries)
    stamp = time.strftime("%Y%m%dT%H%M%S") + "-" + uuid.uuid4().hex[:6]
    result = {"baseline": baseline_id, "candidate": candidate_id,
              "fixture_count": len(baseline), "state": "running"}
    runroot = evidence/("run-"+stamp)
    runroot.mkdir()
    save_json(runroot/"summary.json", result)
    try:
        result["baseline_go"] = run_go_suite(args.baseline, corpus, binaries, cache, runroot/"baseline")
        if args.candidate:
            candidate = {name: observe(args.candidate, corpus, name) for name in baseline}
            save_json(runroot/"candidate-observations.json", candidate)
            differences = compare_observations(baseline, candidate)
            save_json(runroot/"differences.json", differences)
            if differences:
                raise RuntimeError("candidate media behavior differs from baseline")
            result["candidate_go"] = run_go_suite(args.candidate, corpus, binaries, cache, runroot/"candidate")
            for source in (runroot/"baseline/tts-artifacts").glob("*.json"):
                target = runroot/"candidate/tts-artifacts"/source.name
                if not target.exists() or json.loads(source.read_text()) != json.loads(target.read_text()):
                    raise RuntimeError("speech decoded audio or semantics differ for " + source.name)
            result["state"] = "passed"
        else:
            result["state"] = "baseline_passed_candidate_pending"
    except Exception as error:
        result["state"], result["error"] = "failed", str(error)
        raise
    finally:
        save_json(runroot/"summary.json", result)
        print(json.dumps({"evidence": str(runroot), **result}, indent=2))
    return result


def main():
    """main parses explicit local image arguments and performs offline qualification."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--candidate")
    parser.add_argument("--baseline-only", action="store_true")
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("--tokenizer-cache", type=Path, required=True)
    parser.add_argument("--reuse", action="store_true")
    parser.add_argument("--rebuild-tests", action="store_true")
    args = parser.parse_args()
    if bool(args.candidate) == bool(args.baseline_only):
        parser.error("provide either --candidate or --baseline-only")
    try:
        qualify(args)
    except (ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
