#!/usr/bin/env python3
"""Prepare and validate the consolidated frontend security dependency update.

This script is intentionally temporary: the pull-request workflow runs it to
produce a reviewed artifact, and the final commit is assembled from that
artifact without retaining this script in the repository.
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import json
import os
import re
import subprocess
import time
import urllib.error
import urllib.request
from collections import defaultdict
from pathlib import Path
from typing import Iterable

ROOT = Path(__file__).resolve().parents[2]
BASELINE_DIR = Path(os.environ.get("FRONTEND_LOCK_BASELINE", "/tmp/frontend-lock-baseline"))
REPORT_PATH = Path(
    os.environ.get(
        "FRONTEND_SECURITY_REPORT",
        "/tmp/frontend-security-validation-report.md",
    )
)

THEMES = ("air", "berry", "modern")

# This is the union of the security-relevant packages requested by PRs #315,
# #325 and #327, plus the native Rollup packages and later 2026 follow-up fixes
# needed to avoid landing versions that have already been superseded by new
# advisories.
TARGET_PACKAGES = {
    "@remix-run/router",
    "ajv",
    "axios",
    "brace-expansion",
    "flatted",
    "immutable",
    "jsonpath",
    "lodash",
    "mdast-util-to-hast",
    "minimatch",
    "node-forge",
    "picomatch",
    "qs",
    "react-router",
    "react-router-dom",
    "rollup",
    "underscore",
    "webpack",
}

MANIFEST_UPDATES = {
    "air": {
        "axios": "^0.33.0",
        "react-router-dom": "^6.30.3",
    },
    "berry": {
        "axios": "^0.33.0",
        "react-router": "6.30.3",
        "react-router-dom": "6.30.3",
    },
    "modern": {
        "axios": "^1.18.0",
        "react-router-dom": "^6.30.3",
    },
}

# Floors are checked only for a matching major line. OSV is queried as a
# second, independent gate for every changed version and every security target
# still present in the resulting lockfiles.
MINIMUM_BY_MAJOR: dict[str, dict[int, tuple[int, int, int]]] = {
    "@remix-run/router": {1: (1, 23, 2)},
    "ajv": {6: (6, 14, 0), 8: (8, 18, 0)},
    "axios": {0: (0, 33, 0), 1: (1, 18, 0)},
    "flatted": {3: (3, 4, 2)},
    "immutable": {4: (4, 3, 8), 5: (5, 1, 5)},
    "jsonpath": {1: (1, 3, 0)},
    "lodash": {4: (4, 18, 0)},
    "mdast-util-to-hast": {13: (13, 2, 1)},
    "minimatch": {
        3: (3, 1, 4),
        4: (4, 2, 5),
        5: (5, 1, 8),
        6: (6, 2, 2),
        7: (7, 4, 8),
        8: (8, 0, 6),
        9: (9, 0, 7),
        10: (10, 2, 3),
    },
    "node-forge": {1: (1, 4, 0)},
    "picomatch": {2: (2, 3, 2), 3: (3, 0, 2), 4: (4, 0, 4)},
    "qs": {6: (6, 14, 2)},
    "react-router": {6: (6, 30, 3)},
    "react-router-dom": {6: (6, 30, 3)},
    "rollup": {2: (2, 80, 0), 3: (3, 30, 0), 4: (4, 59, 0)},
    "webpack": {5: (5, 105, 4)},
}

EXPECTED_FINAL_PATHS = {
    ".github/workflows/lint.yml",
    "Dockerfile",
    "web/air/package.json",
    "web/air/yarn.lock",
    "web/berry/package.json",
    "web/berry/yarn.lock",
    "web/modern/package.json",
    "web/modern/yarn.lock",
}

TEMPORARY_PATHS = {
    ".github/scripts/prepare_frontend_security_update.py",
    ".github/workflows/prepare-frontend-security-update.yml",
    ".github/workflows/run-frontend-security-update.yml",
}


def selector_package(selector: str) -> str:
    """Return the npm package name from a Yarn Classic lock selector."""

    selector = selector.strip().strip('"')
    if selector.startswith("@"):
        slash = selector.find("/")
        separator = selector.find("@", slash + 1)
    else:
        separator = selector.find("@")
    return selector if separator < 0 else selector[:separator]


def parse_header(line: str) -> list[str]:
    try:
        return next(csv.reader([line[:-1]], skipinitialspace=True))
    except csv.Error as exc:
        raise SystemExit(f"cannot parse Yarn lock header {line!r}: {exc}") from exc


def is_lock_entry(block: str) -> bool:
    lines = block.splitlines()
    return bool(lines and not lines[0].startswith("#") and lines[0].endswith(":"))


def should_refresh(package: str) -> bool:
    return package in TARGET_PACKAGES or package.startswith("@rollup/rollup-")


def parse_lock(path: Path) -> dict[str, set[str]]:
    packages: dict[str, set[str]] = defaultdict(set)
    for block in path.read_text().split("\n\n"):
        if not is_lock_entry(block):
            continue
        lines = block.splitlines()
        version = None
        for line in lines[1:]:
            match = re.fullmatch(r'  version "([^"]+)"', line)
            if match:
                version = match.group(1)
                break
        if version is None:
            raise SystemExit(f"{path}: no version found for lock entry {lines[0]!r}")
        for selector in parse_header(lines[0]):
            packages[selector_package(selector)].add(version)
    return packages


def semver_tuple(version: str) -> tuple[int, int, int]:
    match = re.fullmatch(r"(\d+)\.(\d+)\.(\d+)(?:[-+].*)?", version)
    if not match:
        raise SystemExit(f"cannot compare non-semver dependency version {version!r}")
    return tuple(int(part) for part in match.groups())


def all_pairs(packages: dict[str, set[str]]) -> set[tuple[str, str]]:
    return {
        (package, version)
        for package, versions in packages.items()
        for version in versions
    }


def refresh() -> None:
    for theme, updates in MANIFEST_UPDATES.items():
        manifest_path = ROOT / "web" / theme / "package.json"
        manifest = json.loads(manifest_path.read_text())
        for package, version in updates.items():
            section = next(
                (
                    candidate
                    for candidate in ("dependencies", "devDependencies")
                    if package in manifest.get(candidate, {})
                ),
                None,
            )
            if section is None:
                raise SystemExit(
                    f"{theme}: expected direct dependency {package!r} was not found"
                )
            manifest[section][package] = version
        manifest_path.write_text(
            json.dumps(manifest, indent=2, ensure_ascii=False) + "\n"
        )

        lock_path = ROOT / "web" / theme / "yarn.lock"
        blocks = lock_path.read_text().split("\n\n")
        kept: list[str] = []
        removed: set[str] = set()
        for block in blocks:
            if not is_lock_entry(block):
                kept.append(block)
                continue
            packages = {
                selector_package(selector)
                for selector in parse_header(block.splitlines()[0])
            }
            if any(should_refresh(package) for package in packages):
                removed.update(packages)
                continue
            kept.append(block)
        lock_path.write_text("\n\n".join(kept).rstrip() + "\n")
        print(f"{theme}: invalidated {', '.join(sorted(removed))}")


def osv_query(candidates: Iterable[tuple[str, str]]) -> list[tuple[str, str, str, str]]:
    ordered = sorted(set(candidates))
    endpoint = "https://api.osv.dev/v1/querybatch"
    findings: list[tuple[str, str, str, str]] = []
    print(f"OSV: checking {len(ordered)} changed or security-target package versions")

    for start in range(0, len(ordered), 100):
        batch = ordered[start : start + 100]
        payload = json.dumps(
            {
                "queries": [
                    {
                        "package": {"name": package, "ecosystem": "npm"},
                        "version": version,
                    }
                    for package, version in batch
                ]
            }
        ).encode()

        response_data = None
        for attempt in range(1, 4):
            try:
                request = urllib.request.Request(
                    endpoint,
                    data=payload,
                    headers={"Content-Type": "application/json"},
                    method="POST",
                )
                with urllib.request.urlopen(request, timeout=45) as response:
                    response_data = json.load(response)
                break
            except (urllib.error.URLError, TimeoutError) as exc:
                if attempt == 3:
                    raise SystemExit(
                        f"OSV query failed after 3 attempts: {exc}"
                    ) from exc
                time.sleep(attempt * 2)

        assert response_data is not None
        results = response_data.get("results", [])
        if len(results) != len(batch):
            raise SystemExit(
                f"OSV returned {len(results)} results for {len(batch)} queries"
            )

        for (package, version), result in zip(batch, results, strict=True):
            for vulnerability in result.get("vulns", []):
                if vulnerability.get("withdrawn"):
                    continue
                findings.append(
                    (
                        package,
                        version,
                        vulnerability.get("id", "unknown"),
                        vulnerability.get("summary", "known vulnerability"),
                    )
                )
    return findings


def verify() -> None:
    if not BASELINE_DIR.is_dir():
        raise SystemExit(f"baseline lock directory does not exist: {BASELINE_DIR}")

    updated_by_theme: dict[str, dict[str, set[str]]] = {}
    baseline_pairs: set[tuple[str, str]] = set()
    updated_pairs: set[tuple[str, str]] = set()

    for theme, expected in MANIFEST_UPDATES.items():
        manifest = json.loads((ROOT / "web" / theme / "package.json").read_text())
        direct = {
            **manifest.get("dependencies", {}),
            **manifest.get("devDependencies", {}),
        }
        for package, required in expected.items():
            actual = direct.get(package)
            if actual != required:
                raise SystemExit(
                    f"{theme}: {package} manifest version is {actual!r}; "
                    f"expected {required!r}"
                )

        updated = parse_lock(ROOT / "web" / theme / "yarn.lock")
        baseline = parse_lock(BASELINE_DIR / f"{theme}.yarn.lock")
        updated_by_theme[theme] = updated
        updated_pairs.update(all_pairs(updated))
        baseline_pairs.update(all_pairs(baseline))

        for package, majors in MINIMUM_BY_MAJOR.items():
            for version in updated.get(package, set()):
                parsed = semver_tuple(version)
                minimum = majors.get(parsed[0])
                if minimum is not None and parsed < minimum:
                    raise SystemExit(
                        f"{theme}: {package} {version} is below patched floor "
                        f"{'.'.join(map(str, minimum))}"
                    )

    package_locks = list((ROOT / "web").glob("*/package-lock.json"))
    if package_locks:
        raise SystemExit(
            "package-lock.json must not be reintroduced; Yarn is the repository "
            f"lockfile authority: {package_locks}"
        )

    changed_pairs = updated_pairs - baseline_pairs
    target_pairs = {
        pair
        for pair in updated_pairs
        if should_refresh(pair[0])
    }
    candidates = changed_pairs | target_pairs
    findings = osv_query(candidates)
    if findings:
        for package, version, vulnerability_id, summary in findings:
            print(f"VULNERABLE: {package}@{version}: {vulnerability_id}: {summary}")
        raise SystemExit(
            "known vulnerabilities remain in changed or security-target dependencies"
        )

    REPORT_PATH.parent.mkdir(parents=True, exist_ok=True)
    lines = [
        "# Frontend security dependency validation",
        "",
        f"Generated: {dt.datetime.now(dt.timezone.utc).isoformat()}",
        "",
        "## Scope",
        "",
        "Consolidates and supersedes dependency-update PRs #315, #325 and #327 ",
        "against the current `main` lockfiles.",
        "",
        "## Direct dependency policy",
        "",
    ]
    for theme in THEMES:
        direct = ", ".join(
            f"`{package}` → `{version}`"
            for package, version in sorted(MANIFEST_UPDATES[theme].items())
        )
        lines.append(f"- **{theme}:** {direct}")

    lines.extend(
        [
            "",
            "## Resolved security-target versions",
            "",
            "| Theme | Package | Versions |",
            "| --- | --- | --- |",
        ]
    )
    for theme in THEMES:
        packages = updated_by_theme[theme]
        for package in sorted(packages):
            if should_refresh(package):
                versions = ", ".join(f"`{version}`" for version in sorted(packages[package]))
                lines.append(f"| {theme} | `{package}` | {versions} |")

    lines.extend(
        [
            "",
            "## Vulnerability gate",
            "",
            f"OSV checked **{len(candidates)}** changed or security-target npm package versions.",
            "No active OSV vulnerability was returned for the checked versions.",
            "",
            "The workflow also requires clean frozen Yarn installs, all three frontend test ",
            "suites, and the Docker `web-builder` production build to pass before publishing ",
            "the generated artifact.",
            "",
        ]
    )
    REPORT_PATH.write_text("\n".join(lines))
    print(f"OSV: no known vulnerabilities found; report written to {REPORT_PATH}")


def harden() -> None:
    lint_path = ROOT / ".github/workflows/lint.yml"
    lint = lint_path.read_text()

    install_old = "run: yarn install --network-timeout 600000"
    install_new = "run: yarn install --frozen-lockfile --network-timeout 600000"
    if lint.count(install_old) != 3:
        raise SystemExit(
            "expected exactly three non-frozen frontend install commands in lint.yml"
        )
    lint = lint.replace(install_old, install_new)

    node_old = "node-version: '20'"
    node_new = "node-version: '24'"
    if lint.count(node_old) != 3:
        raise SystemExit("expected exactly three Node 20 frontend jobs in lint.yml")
    lint = lint.replace(node_old, node_new)

    test_commands = {
        "modern": "run: yarn test --run --passWithNoTests",
        "air": "run: yarn test --watchAll=false --passWithNoTests",
        "berry": "run: yarn test --watchAll=false --passWithNoTests",
    }
    for theme, command in test_commands.items():
        marker = f"        {command}\n"
        if lint.count(marker) != 1:
            raise SystemExit(f"expected one {theme} test command in lint.yml")
        replacement = (
            marker
            + "\n"
            + "      - name: Build production bundle\n"
            + f"        working-directory: web/{theme}\n"
            + "        env:\n"
            + "          DISABLE_ESLINT_PLUGIN: 'true'\n"
            + "          REACT_APP_VERSION: ci\n"
            + "        run: yarn build\n"
        )
        lint = lint.replace(marker, replacement)
    lint_path.write_text(lint)

    docker_path = ROOT / "Dockerfile"
    docker = docker_path.read_text()
    docker_old = "yarn install --network-timeout 600000"
    docker_new = "yarn install --frozen-lockfile --network-timeout 600000"
    if docker.count(docker_old) != 1:
        raise SystemExit(
            "expected exactly one non-frozen frontend install command in Dockerfile"
        )
    docker_path.write_text(docker.replace(docker_old, docker_new))


def changed_paths() -> set[str]:
    output = subprocess.check_output(
        ["git", "diff", "--name-only", "origin/main"],
        cwd=ROOT,
        text=True,
    )
    return set(output.splitlines())


def validate() -> None:
    changed = changed_paths()
    effective = changed - TEMPORARY_PATHS
    unexpected = effective - EXPECTED_FINAL_PATHS
    missing = EXPECTED_FINAL_PATHS - effective
    if unexpected:
        raise SystemExit(f"unexpected generated changes: {sorted(unexpected)}")
    if missing:
        raise SystemExit(f"expected generated changes are missing: {sorted(missing)}")

    package_locks = list((ROOT / "web").glob("*/package-lock.json"))
    if package_locks:
        raise SystemExit(f"unexpected npm lockfiles: {package_locks}")

    subprocess.run(["git", "diff", "--check", "origin/main"], cwd=ROOT, check=True)
    print("validated final generated paths:")
    for path in sorted(effective):
        print(f"  {path}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("refresh", "verify", "harden", "validate"))
    args = parser.parse_args()

    if args.command == "refresh":
        refresh()
    elif args.command == "verify":
        verify()
    elif args.command == "harden":
        harden()
    else:
        validate()


if __name__ == "__main__":
    main()
