#!/usr/bin/env python3
"""Check tracked Go formatting without modifying source or mistaking CRLF for drift."""
from __future__ import annotations

from pathlib import Path
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[2]


def check_files(root: Path, files: list[str]) -> None:
    """check_files checks a nonempty Go inventory and raises on formatter failure or drift."""
    if not files or len(files) != len(set(files)):
        raise ValueError("Go formatting inventory is empty or duplicated")
    started = time.monotonic()
    sources = {}
    for name in files:
        relative = Path(name)
        if relative.is_absolute() or ".." in relative.parts or relative.suffix != ".go":
            raise ValueError(f"Invalid formatting path: {name}")
        sources[name] = (root / relative).read_bytes()
    # Normal Linux checkouts need no copies. Only CRLF input uses temporary files.
    if not any(b"\r\n" in source for source in sources.values()):
        for offset in range(0, len(files), 128):
            result = subprocess.run(["gofmt", "-l", *files[offset:offset + 128]], cwd=root,
                                    capture_output=True, text=True, timeout=120, check=True)
            if result.stdout.strip():
                raise ValueError("Go files require gofmt:\n" + result.stdout.strip())
        print(f"Go formatting passed for {len(files)} tracked files in {time.monotonic() - started:.3f}s.")
        return
    with tempfile.TemporaryDirectory(prefix="oneapi-gofmt-") as temporary:
        destination = Path(temporary)
        for name in files:
            relative = Path(name)
            target = destination / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(sources[name].replace(b"\r\n", b"\n"))
        result = subprocess.run(["gofmt", "-l", "."], cwd=destination,
                                capture_output=True, text=True, timeout=120, check=True)
        if result.stdout.strip():
            raise ValueError("Go files require gofmt:\n" + result.stdout.strip())
    print(f"Go formatting passed for {len(files)} tracked files in {time.monotonic() - started:.3f}s.")


def main() -> None:
    """main discovers tracked Go paths and propagates missing source, process, and format errors."""
    result = subprocess.run(["git", "ls-files", "-z", "--", "*.go"], cwd=ROOT,
                            capture_output=True, check=True, timeout=30)
    files = [name.decode("utf-8") for name in result.stdout.split(b"\0") if name]
    check_files(ROOT, files)


if __name__ == "__main__":
    main()
