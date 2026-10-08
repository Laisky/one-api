#!/usr/bin/env python3
"""Run the essential unit gate and retain fail-closed discovery/execution evidence."""

import argparse
import collections
import json
from pathlib import Path
import re
import subprocess
import sys
import time

MODULE = "github.com/Laisky/one-api"
# None means every currently discoverable top-level unit test in this package.
# Keep model and adapter suites in manual full qualification: their measured clean
# compilation cost left insufficient margin for a complete CI job below five minutes.
SELECTIONS = {
    "relay/format": None,
}


def parse_events(output):
    """parse_events returns strict Go JSON events or raises for malformed evidence."""
    events = []
    for line in output.splitlines():
        if not line.strip():
            continue
        event = json.loads(line)
        if not isinstance(event, dict) or not isinstance(event.get("Action"), str):
            raise ValueError("invalid Go event")
        events.append(event)
    if not events:
        raise ValueError("empty Go event stream")
    return events


def discover(events, selections=SELECTIONS):
    """discover validates successful package inventory and returns exact selected tests."""
    inventory = {f"{MODULE}/{package}": [] for package in selections}
    terminal = collections.Counter()
    for event in events:
        package = event.get("Package")
        if event["Action"] in ("fail", "build-fail", "skip"):
            raise ValueError("test discovery failed or skipped")
        if package not in inventory:
            continue
        if event["Action"] == "pass" and not event.get("Test"):
            terminal[package] += 1
        if event["Action"] == "output":
            for line in event.get("Output", "").splitlines():
                if re.fullmatch(r"Test[A-Za-z0-9_]+", line):
                    inventory[package].append(line)
    selected = {}
    for short_package, requested in selections.items():
        package = f"{MODULE}/{short_package}"
        names = inventory[package]
        if terminal[package] != 1 or not names or len(names) != len(set(names)):
            raise ValueError(f"missing, duplicate, or incomplete inventory: {package}")
        expected = names if requested is None else list(requested)
        missing = set(expected) - set(names)
        if missing:
            raise ValueError(f"required tests absent in {package}: {sorted(missing)}")
        selected[package] = sorted(expected)
    return selected, inventory


def validate_execution(events, selected):
    """validate_execution rejects failures, skips, missing or duplicate top-level results."""
    runs = collections.Counter()
    passes = collections.Counter()
    packages = collections.Counter()
    for event in events:
        action = event["Action"]
        package = event.get("Package")
        test = event.get("Test")
        if action in ("fail", "build-fail", "skip"):
            raise ValueError(f"failed or skipped execution: {package} {test or ''}")
        if package not in selected:
            if test or action == "pass":
                raise ValueError(f"unexpected test package: {package}")
            continue
        if test and "/" not in test:
            if test not in selected[package]:
                raise ValueError(f"unexpected top-level test: {package} {test}")
            if action == "run":
                runs[package, test] += 1
            elif action == "pass":
                passes[package, test] += 1
        elif action == "pass" and not test:
            packages[package] += 1
    for package, names in selected.items():
        if packages[package] != 1:
            raise ValueError(f"missing or duplicate successful package result: {package}")
        for name in names:
            if runs[package, name] != 1 or passes[package, name] != 1:
                raise ValueError(f"missing or duplicate successful test: {package} {name}")
    return {package: len(names) for package, names in selected.items()}


def run_go(go, arguments, output, label, deadline):
    """run_go saves raw process evidence and enforces the shared command deadline."""
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        raise TimeoutError("quick gate exceeded wall-clock budget")
    started = time.monotonic()
    command = [go, "test", *arguments]
    try:
        result = subprocess.run(command, capture_output=True, text=True,
                                encoding="utf-8", errors="replace", timeout=remaining)
    except subprocess.TimeoutExpired as error:
        # TimeoutExpired may carry bytes even when text=True. Retain partial evidence.
        for suffix, content in (("jsonl", error.stdout), ("stderr.txt", error.stderr)):
            if isinstance(content, bytes):
                content = content.decode("utf-8", errors="replace")
            (output / f"{label}.{suffix}").write_text(content or "", encoding="utf-8")
        receipt = {"command": command, "exit_code": None, "status": "timed_out",
                   "elapsed_seconds": round(time.monotonic() - started, 3)}
        (output / f"{label}.receipt.json").write_text(json.dumps(receipt, indent=2) + "\n",
                                                     encoding="utf-8")
        raise
    duration = time.monotonic() - started
    (output / f"{label}.jsonl").write_text(result.stdout, encoding="utf-8")
    (output / f"{label}.stderr.txt").write_text(result.stderr, encoding="utf-8")
    receipt = {"command": command, "exit_code": result.returncode,
               "elapsed_seconds": round(duration, 3)}
    (output / f"{label}.receipt.json").write_text(json.dumps(receipt, indent=2) + "\n",
                                                 encoding="utf-8")
    if result.returncode:
        raise ValueError(f"{label} exited {result.returncode}; see {output}")
    return parse_events(result.stdout), receipt


def main():
    """main runs discovered essential tests and writes an honest pass or failure summary."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default="go")
    parser.add_argument("--output", default="quick-test-results")
    parser.add_argument("--wall-timeout", type=int, default=240)
    args = parser.parse_args()
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    started = time.monotonic()
    summary = {"status": "failed", "scope": "essential unit tests", "receipts": []}
    try:
        deadline = started + args.wall_timeout
        packages = [f"./{package}" for package in SELECTIONS]
        events, receipt = run_go(args.go, ["-json", "-list", "^Test", *packages],
                                 output, "discovery", deadline)
        summary["receipts"].append(receipt)
        selected, inventory = discover(events)
        summary["discovered_counts"] = {package: len(names)
                                         for package, names in inventory.items()}
        summary["selected"] = selected
        names = sorted({name for tests in selected.values() for name in tests})
        expression = "^(?:" + "|".join(re.escape(name) for name in names) + ")$"
        events, receipt = run_go(args.go, ["-json", "-count=1", "-timeout=2m",
                                          "-run", expression, *packages],
                                 output, "execution", deadline)
        summary["receipts"].append(receipt)
        summary["passed_counts"] = validate_execution(events, selected)
        summary["status"] = "passed"
    except (ValueError, OSError, subprocess.TimeoutExpired, TimeoutError) as error:
        summary["error"] = str(error)
    summary["elapsed_seconds"] = round(time.monotonic() - started, 3)
    (output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n",
                                          encoding="utf-8")
    print(json.dumps(summary, indent=2))
    return 0 if summary["status"] == "passed" else 1


if __name__ == "__main__":
    sys.exit(main())
