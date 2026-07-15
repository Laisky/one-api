#!/usr/bin/env python3
"""Diff two recordings produced by journey.py / relay_journey.py.

Exits non-zero when the two builds disagree, so this is usable as a gate.

Usage:
    compare.py <baseline.json> <candidate.json>
    compare.py --self-test <recording.json>

--self-test injects synthetic changes into a recording and asserts they are all
caught. A differential that cannot fail proves nothing, and this gate is easy to
render vacuous by over-normalizing, so the check is built in rather than left to
good intentions.
"""

import copy
import json
import sys


def diff(baseline, candidate):
    """Compare two recordings.

    Parameters:
      - baseline: recording dict from the reference build.
      - candidate: recording dict from the build under test.

    Return values:
      - list[str]: human-readable differences; empty means identical.
    """
    problems = []
    base_steps, cand_steps = baseline["steps"], candidate["steps"]

    only_base = sorted(set(base_steps) - set(cand_steps))
    only_cand = sorted(set(cand_steps) - set(base_steps))
    for name in only_base:
        problems.append(f"step missing from candidate: {name}")
    for name in only_cand:
        problems.append(f"step only in candidate: {name}")

    for name in sorted(set(base_steps) & set(cand_steps)):
        want, got = base_steps[name], cand_steps[name]
        if want["status"] != got["status"]:
            problems.append(
                f"STATUS {name}: {want['status']} -> {got['status']}")
        if want.get("shape") != got.get("shape"):
            want_shape, got_shape = want.get("shape") or [], got.get("shape") or []
            for entry in [e for e in want_shape if e not in got_shape]:
                problems.append(f"SHAPE {name}: only in baseline: {entry}")
            for entry in [e for e in got_shape if e not in want_shape]:
                problems.append(f"SHAPE {name}: only in candidate: {entry}")

    # Any secret in either recording is a failure regardless of agreement: two
    # builds leaking the same secret is not a passing result.
    for tag, rec in (("baseline", baseline), ("candidate", candidate)):
        for leak in rec.get("secret_leaks") or []:
            problems.append(f"SECRET LEAK in {tag}: {leak}")

    if baseline.get("quota_delta") != candidate.get("quota_delta"):
        problems.append(
            f"BILLING quota_delta: {baseline.get('quota_delta')} -> "
            f"{candidate.get('quota_delta')}")
    return problems


def self_test(recording):
    """Assert the differ catches injected changes; return True when it does."""
    checks = []

    mutated = copy.deepcopy(recording)
    name = sorted(mutated["steps"])[0]
    mutated["steps"][name]["status"] = 599
    checks.append(("status change", bool(diff(recording, mutated))))

    mutated = copy.deepcopy(recording)
    for name in sorted(mutated["steps"]):
        if "shape" in mutated["steps"][name]:
            mutated["steps"][name]["shape"].append(".data.id=42")
            break
    checks.append(("leaked id key", bool(diff(recording, mutated))))

    mutated = copy.deepcopy(recording)
    mutated["steps"].pop(sorted(mutated["steps"])[0])
    checks.append(("dropped step", bool(diff(recording, mutated))))

    mutated = copy.deepcopy(recording)
    mutated["secret_leaks"] = ['fake_step: "password"']
    checks.append(("secret leak", bool(diff(recording, mutated))))

    ok = True
    for label, caught in checks:
        print(f"  {'PASS' if caught else 'FAIL'}  differ catches {label}")
        ok = ok and caught
    return ok


def main():
    if sys.argv[1:2] == ["--self-test"]:
        with open(sys.argv[2], encoding="utf-8") as handle:
            recording = json.load(handle)
        print("self-test: the differ must catch every injected change")
        if not self_test(recording):
            print("\nSELF-TEST FAILED: the differ is vacuous")
            return 1
        print("\nself-test passed")
        return 0

    if len(sys.argv) != 3:
        print(__doc__)
        return 2

    with open(sys.argv[1], encoding="utf-8") as handle:
        baseline = json.load(handle)
    with open(sys.argv[2], encoding="utf-8") as handle:
        candidate = json.load(handle)

    problems = diff(baseline, candidate)
    if not problems:
        steps = len(candidate["steps"])
        print(f"IDENTICAL: {steps} steps agree; no secret leaked; billing equal.")
        return 0

    print(f"{len(problems)} DIFFERENCE(S):")
    for problem in problems:
        print(f"  {problem}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
