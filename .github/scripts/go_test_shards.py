#!/usr/bin/env python3
"""Run isolated Go test shards and validate their combined inventory and coverage.

Only model's top-level tests are partitioned. Subtests, fuzz seeds, race
instrumentation and real database scenarios stay intact. All other packages
run in a separate job with their normal Go package-level concurrency.
"""
from __future__ import annotations

import argparse
from collections import Counter
import json
import math
import os
from pathlib import Path
import re
import subprocess
import sys
import time

MODEL_SHARDS = 4
SHARDS = ("packages", *(f"model-{i}" for i in range(MODEL_SHARDS)))
TEST_NAME = re.compile(r"(?:Test|Fuzz|Example)\w*\Z", re.UNICODE)
COVER_LINE = re.compile(r"(.+):([0-9]+\.[0-9]+,[0-9]+\.[0-9]+) ([0-9]+) ([0-9]+)\Z")
FLAGS = ["-race", "-cover", "-covermode=atomic", "-count=1", "-timeout=45m"]
ROOT = Path(__file__).resolve().parents[2]


def partition_tests(names: list[str], weights: dict[str, float]) -> list[list[str]]:
    """partition_tests assigns every current top-level name exactly once, deterministically."""
    if len(names) != len(set(names)) or not names:
        raise ValueError("Test inventory is empty or contains duplicate names")
    if any(TEST_NAME.fullmatch(name) is None for name in names):
        raise ValueError("Test inventory contains an invalid top-level name")
    if any(not math.isfinite(value) or value < 0 for value in weights.values()):
        raise ValueError("Timing hints must be finite nonnegative numbers")
    groups: list[list[str]] = [[] for _ in range(MODEL_SHARDS)]
    loads = [0.0] * MODEL_SHARDS
    for name in sorted(names, key=lambda item: (-weights.get(item, 1.0), item)):
        index = min(range(MODEL_SHARDS), key=lambda i: (loads[i], i))
        groups[index].append(name)
        loads[index] += weights.get(name, 1.0)
    # Keep Go's normal source ordering during execution; this only sorts the selector.
    return [sorted(group) for group in groups]


def selector(names: list[str]) -> str:
    """selector builds an anchored Go regexp without filtering any child subtest."""
    if not names or any(TEST_NAME.fullmatch(name) is None for name in names):
        raise ValueError("Refusing an empty or invalid test selector")
    return "^(" + "|".join(names) + ")$"


def go_output(*args: str) -> str:
    """go_output runs a bounded discovery command and propagates compilation failures."""
    result = subprocess.run(["go", *args], cwd=ROOT, text=True,
                            stdout=subprocess.PIPE, stderr=None, timeout=900, check=True)
    return result.stdout


def discover(shard: str) -> dict:
    """discover records all current packages and the exact model selection for one shard."""
    packages = sorted(go_output("list", "./...").splitlines())
    module = go_output("list", "-m").strip()
    model = module + "/model"
    if not packages or len(packages) != len(set(packages)) or model not in packages:
        raise ValueError("Invalid package inventory or missing model package")
    names: list[str] = []
    chosen: list[str] = []
    selected_packages = [package for package in packages if package != model]
    if shard != "packages":
        # -list compiles but does not run tests. Matching instrumentation reuses the
        # compilation cache on the subsequent real run; no cached test result is used.
        listing = go_output("test", *FLAGS, "-list=.", model)
        names = [line for line in listing.splitlines() if TEST_NAME.fullmatch(line)]
        weights = json.loads((Path(__file__).with_name("go_test_weights.json")).read_text())
        chosen = partition_tests(names, weights["model_seconds"])[int(shard.removeprefix("model-"))]
        selector(chosen)  # Fail rather than turning an empty selection into "run everything".
        selected_packages = [model]
    return {"shard": shard, "revision": os.environ.get("GITHUB_SHA", "local"),
            "all_packages": packages, "model_package": model,
            "model_inventory": sorted(names), "selected_tests": chosen,
            "selected_packages": selected_packages}


def read_events(path: Path) -> list[dict]:
    """read_events loads Go JSON events and rejects corrupt or truncated evidence."""
    events = []
    with path.open() as source:
        for line in source:
            event = json.loads(line)
            if not isinstance(event, dict) or not isinstance(event.get("Action"), str):
                raise ValueError("Malformed Go JSON event")
            events.append(event)
    if not events:
        raise ValueError("Go JSON evidence is empty")
    return events


def validate_events(manifest: dict, events: list[dict]) -> None:
    """validate_events requires successful package completion and complete model execution."""
    packages: Counter = Counter()
    terminals: Counter = Counter()
    starts: Counter = Counter()
    for event in events:
        action, package, test = event.get("Action"), event.get("Package"), event.get("Test")
        if action in {"fail", "build-fail"}:
            raise ValueError(f"Go reported failure: {package or event.get('ImportPath')} {test or ''}")
        if package and not test and action in {"pass", "skip"}:
            packages[package] += 1
        if package == manifest["model_package"] and test and "/" not in test:
            if action == "run":
                starts[test] += 1
            if action in {"pass", "skip"}:
                terminals[test] += 1
    if packages != Counter(manifest["selected_packages"]):
        raise ValueError("Executed package inventory differs from the selected packages")
    if manifest["shard"] != "packages":
        expected = Counter(manifest["selected_tests"])
        if starts != expected or terminals != expected:
            raise ValueError("A selected model test was missing, duplicated or incomplete")


def markdown_text(value: str) -> str:
    """markdown_text escapes test names before including them in a job summary."""
    return value.replace("&", "&amp;").replace("<", "&lt;").replace(
        ">", "&gt;").replace("|", "&#124;").replace("`", "&#96;").replace("\n", " ")


def summarize(events: list[dict], shard: str, elapsed: float, status: int) -> str:
    """summarize reports slow top-level tests separately from nested subtests and skips."""
    finished = [event for event in events if event.get("Test") and
                event.get("Action") in {"pass", "fail", "skip"}]
    top = [event for event in finished if "/" not in event["Test"]]
    counts = Counter(event["Action"] for event in top)
    lines = [f"## Go tests: {markdown_text(shard)}", "",
             f"Exit status: **{status}**. Test command wall time: **{elapsed:.2f}s**.",
             f"Top-level results: {counts['pass']} passed, {counts['fail']} failed, {counts['skip']} skipped.",
             "", "| Slowest top-level tests | Seconds |", "| --- | ---: |"]
    for event in sorted(top, key=lambda item: item.get("Elapsed", 0), reverse=True)[:15]:
        name = markdown_text(event.get("Package", "") + "/" + event["Test"])
        lines.append(f"| {name} | {event.get('Elapsed', 0):.2f} |")
    skips = [event for event in finished if event["Action"] == "skip"]
    failures = [event for event in finished if event["Action"] == "fail"]
    for label, selected in (("Failures", failures), ("Skipped tests (including subtests)", skips)):
        if selected:
            lines.extend(["", f"### {label}", ""])
            lines.extend(markdown_text(event.get("Package", "") + "/" + event["Test"]) + "  "
                         for event in selected[:80])
            if len(selected) > 80:
                lines.append(f"{len(selected) - 80} more; see the complete JSON artifact.")
    return "\n".join(lines) + "\n"


def run_shard(shard: str, output: Path) -> int:
    """run_shard streams quiet progress, retains complete logs, and preserves Go's exit code."""
    output.mkdir(parents=True, exist_ok=True)
    manifest = discover(shard)
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")
    command = ["go", "test", *FLAGS, "-json", "-coverprofile=" + str(output / "coverage.txt")]
    if shard != "packages":
        command += ["-run=" + selector(manifest["selected_tests"])]
    command += manifest["selected_packages"]
    print(f"Running {shard}: {len(manifest['selected_packages'])} packages; "
          f"{len(manifest['selected_tests'])} selected model tests", flush=True)
    start = time.monotonic()
    with (output / "events.jsonl").open("w") as raw, (output / "go-tests.log").open("w") as human:
        # stderr remains visible for build/driver diagnostics, but never pollutes JSON.
        with subprocess.Popen(command, cwd=ROOT, stdout=subprocess.PIPE, text=True,
                              encoding="utf-8", errors="replace", bufsize=1) as process:
            assert process.stdout is not None
            for line in process.stdout:
                raw.write(line)
                raw.flush()
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue  # Complete validation below rejects malformed streams.
                human.write(event.get("Output", ""))
                if event.get("Action") in {"pass", "fail", "skip"} and not event.get("Test"):
                    print(f"{event['Action']}: {event.get('Package', '')} "
                          f"{event.get('Elapsed', 0):.2f}s", flush=True)
                elif (event.get("Action") == "pass" and event.get("Test") and
                      "/" not in event["Test"] and event.get("Elapsed", 0) >= 5):
                    print(f"PASS: {event.get('Package', '')}/{event['Test']} "
                          f"{event['Elapsed']:.2f}s", flush=True)
                elif event.get("Action") == "fail":
                    print(f"FAIL: {event.get('Package', '')} {event.get('Test', '')}", flush=True)
            status = process.wait()
    elapsed = time.monotonic() - start
    events = []
    try:
        events = read_events(output / "events.jsonl")
        validate_events(manifest, events)
        if not (output / "coverage.txt").is_file():
            raise ValueError("Coverage profile is missing")
    except (ValueError, OSError) as exc:
        print(f"Evidence validation failed: {exc}", file=sys.stderr)
        status = status or 1
    manifest.update(exit_code=status, elapsed_seconds=elapsed)
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")
    summary = summarize(events, shard, elapsed, status)
    (output / "summary.md").write_text(summary)
    summary_path = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary_path:
        with open(summary_path, "a") as target:
            target.write(summary)
    return status


def merge_coverage(profiles: list[Path], destination: Path) -> None:
    """merge_coverage sums atomic counters and counts each statement block only once."""
    blocks: dict[tuple[str, str], tuple[int, int]] = {}
    for profile in profiles:
        with profile.open() as source:
            if source.readline().strip() != "mode: atomic":
                raise ValueError(f"Expected atomic coverage: {profile}")
            seen = set()
            for line in source:
                match = COVER_LINE.fullmatch(line.rstrip("\n"))
                if match is None:
                    raise ValueError(f"Malformed coverage block in {profile}")
                filename, position, statements, count = match.groups()
                key = (filename, position)
                if key in seen:
                    raise ValueError(f"Duplicate block within {profile}")
                seen.add(key)
                statements, count = int(statements), int(count)
                old_statements, old_count = blocks.get(key, (statements, 0))
                if old_statements != statements:
                    raise ValueError("Conflicting statement counts for the same coverage block")
                blocks[key] = (statements, old_count + count)
            if not seen:
                raise ValueError(f"Empty coverage profile: {profile}")
    if not blocks:
        raise ValueError("No coverage profiles to merge")
    destination.write_text("mode: atomic\n" + "".join(
        f"{filename}:{position} {statements} {count}\n"
        for (filename, position), (statements, count) in sorted(blocks.items())))


def merge_shards(directory: Path, destination: Path) -> None:
    """merge_shards rejects incomplete shard evidence before publishing combined coverage."""
    paths = sorted(directory.glob("*/manifest.json"))
    manifests = [json.loads(path.read_text()) for path in paths]
    if Counter(item["shard"] for item in manifests) != Counter(SHARDS):
        raise ValueError("Expected exactly one artifact for every Go shard")
    first = manifests[0]
    selected = []
    model_inventory = None
    for manifest, path in zip(manifests, paths):
        if manifest.get("exit_code") != 0:
            raise ValueError(f"Shard did not pass: {manifest['shard']}")
        for key in ("revision", "all_packages", "model_package"):
            if manifest[key] != first[key]:
                raise ValueError(f"Shards disagree on {key}")
        if manifest["revision"] != os.environ.get("GITHUB_SHA", "local"):
            raise ValueError("Shard evidence is from another revision")
        packages = manifest["all_packages"]
        model = manifest["model_package"]
        if len(packages) != len(set(packages)) or model not in packages:
            raise ValueError("Invalid complete package inventory")
        expected_packages = [p for p in packages if p != model] if manifest["shard"] == "packages" else [model]
        if manifest["selected_packages"] != expected_packages:
            raise ValueError("Package selection omitted or repeated a package")
        if manifest["shard"] != "packages":
            inventory = manifest["model_inventory"]
            if not inventory or len(inventory) != len(set(inventory)):
                raise ValueError("Invalid model inventory")
            if model_inventory is None:
                model_inventory = inventory
            if inventory != model_inventory:
                raise ValueError("Model test inventories differ")
            selected.extend(manifest["selected_tests"])
        validate_events(manifest, read_events(path.parent / "events.jsonl"))
    if Counter(selected) != Counter(model_inventory or []):
        raise ValueError("Model tests were omitted or assigned to multiple shards")
    merge_coverage([path.parent / "coverage.txt" for path in paths], destination)
    message = f"Verified {len(first['all_packages'])} packages and {len(selected)} model tests; coverage merged."
    print(message)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as target:
            target.write("## Go test completeness\n\n" + message + "\n\n")
            target.write("| Shard | Test command wall time |\n| --- | ---: |\n")
            for item in sorted(manifests, key=lambda item: item["shard"]):
                target.write(f"| {item['shard']} | {item.get('elapsed_seconds', 0):.2f}s |\n")


def main() -> int:
    """main dispatches the runner or completeness-checked coverage merger."""
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    run = sub.add_parser("run")
    run.add_argument("--shard", choices=SHARDS, required=True)
    run.add_argument("--output", type=Path, required=True)
    merge = sub.add_parser("merge")
    merge.add_argument("--input", type=Path, required=True)
    merge.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.command == "run":
        return run_shard(args.shard, args.output.resolve())
    merge_shards(args.input, args.output)
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print(f"Go test orchestration failed: {error}", file=sys.stderr)
        sys.exit(1)
