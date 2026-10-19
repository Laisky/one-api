#!/usr/bin/env python3
"""Collect unchanged FFmpeg tools, their ELF dependencies and runtime modules.

Run only inside the trusted media build stage. The generated tree is copied into
the final image; no application source, credentials or build caches are included.
"""
import glob
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess

ROOT = Path("/media-root")
MEDIA = ROOT / "opt/oneapi-media"
LIB = MEDIA / "lib"
RECORDS = []
COPIED = {}
ELF_FILES = {}
DANGLING_DOC_LINKS = []
DATA_FILES = []


def digest(path):
    """digest returns a file's SHA256 without retaining its content in memory."""
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def elf_metadata(path):
    """elf_metadata reads architecture, interpreter and dynamic lookup constraints."""
    with path.open("rb") as stream:
        header = stream.read(64)
        if header[:5] != b"\x7fELF\x02":
            raise ValueError(f"expected a 64-bit ELF: {path}")
        endian = "<" if header[5] == 1 else ">"
        machine = struct.unpack_from(endian + "H", header, 18)[0]
        phoff = struct.unpack_from(endian + "Q", header, 32)[0]
        shoff = struct.unpack_from(endian + "Q", header, 40)[0]
        phsize, phnum, shsize, shnum = struct.unpack_from(endian + "HHHH", header, 54)
        interpreter = None
        for index in range(phnum):
            stream.seek(phoff + index * phsize)
            segment = struct.unpack(endian + "IIQQQQQQ", stream.read(phsize))
            if segment[0] == 3:
                stream.seek(segment[2])
                interpreter = stream.read(segment[5]).rstrip(b"\0").decode()
        stream.seek(shoff)
        sections = [struct.unpack(endian + "IIQQQQIIQQ", stream.read(shsize))
                    for _ in range(shnum)]
        needed, lookup_paths = [], []
        for section in sections:
            if section[1] != 6:
                continue
            strings = sections[section[6]]
            stream.seek(strings[4])
            names = stream.read(strings[5])
            stream.seek(section[4])
            dynamic = stream.read(section[5])
            for offset in range(0, len(dynamic), 16):
                tag, value = struct.unpack_from(endian + "qQ", dynamic, offset)
                if tag not in (1, 15, 29):
                    continue
                name = names[value:].split(b"\0")[0].decode()
                (needed if tag == 1 else lookup_paths).append(name)
        if any("/" in name for name in needed):
            raise ValueError(f"absolute dependency lookup in {path}: {needed}")
        return {"machine": machine, "interpreter": interpreter,
                "needed": needed, "lookup_paths": lookup_paths}


def validate_lookup(metadata, destination):
    """validate_lookup permits only origin-relative paths confined to the private tree."""
    for paths in metadata["lookup_paths"]:
        for value in paths.split(":"):
            if not re.match(r"^(?:\$ORIGIN|\$\{ORIGIN\})(?:/|$)", value):
                raise ValueError(f"non-relocatable lookup in {destination}: {value}")
            value = value.replace("${ORIGIN}", str(destination.parent))
            value = value.replace("$ORIGIN", str(destination.parent))
            resolved = Path(os.path.normpath(value))
            if "$" in value or not resolved.is_relative_to(MEDIA):
                raise ValueError(f"dependency lookup escapes private tree: {value}")
    ELF_FILES[str(destination.relative_to(ROOT))] = metadata


def copy_file(source, destination, *, role):
    """copy_file preserves file bytes and rejects conflicting destination aliases."""
    source = Path(source)
    checksum = digest(source)
    key = str(destination)
    if key in COPIED:
        if COPIED[key] != checksum:
            raise ValueError(f"different libraries share alias {destination.name}")
        return
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, destination, follow_symlinks=True)
    destination.chmod(0o755 if role in ("program", "loader") else 0o644)
    COPIED[key] = checksum
    RECORDS.append({"source": str(source), "realpath": str(source.resolve()),
                    "destination": str(destination.relative_to(ROOT)),
                    "sha256": checksum, "role": role})


def dependencies(path):
    """dependencies returns ldd's requested SONAME and resolved path pairs."""
    result = subprocess.run(["ldd", str(path)], text=True, capture_output=True, check=True)
    if "not found" in result.stdout:
        raise ValueError(f"unresolved dependencies for {path}")
    resolved = []
    for line in result.stdout.splitlines():
        match = re.match(r"\s*(\S+)\s+=>\s+(/\S+)", line)
        if match:
            resolved.append((match[1], Path(match[2])))
            continue
        match = re.match(r"\s*(/\S+)\s+\(", line)
        if match:
            source = Path(match[1])
            resolved.append((source.name, source))
    return resolved


def collect_elf(path, destination, *, role):
    """collect_elf copies one ELF and its complete ldd closure with alias integrity."""
    metadata = elf_metadata(path)
    if metadata["machine"] != MACHINE:
        raise ValueError(f"mixed ELF architecture: {path}")
    validate_lookup(metadata, destination)
    copy_file(path, destination, role=role)
    for alias, source in dependencies(path):
        details = elf_metadata(source)
        if details["machine"] != MACHINE:
            raise ValueError(f"mixed ELF architecture: {source}")
        validate_lookup(details, LIB / alias)
        copy_file(source, LIB / alias, role="library")


def copy_tree(source, destination, *, documentation=False):
    """copy_tree copies public runtime data and notices, dereferencing source links."""
    def ignore_missing_docs(directory, names):
        """ignore_missing_docs records already-broken non-license documentation links."""
        ignored = []
        for name in names:
            path = Path(directory) / name
            if not path.is_symlink() or path.exists():
                continue
            if any(word in name.lower() for word in ("copyright", "license", "copying", "notice")):
                raise ValueError(f"source notice target is missing: {path}")
            DANGLING_DOC_LINKS.append({"source": str(path), "target": os.readlink(path)})
            ignored.append(name)
        return ignored

    if source.is_dir():
        shutil.copytree(source, destination, symlinks=False, dirs_exist_ok=True,
                        ignore=ignore_missing_docs if documentation else None)
        for output in sorted(destination.rglob("*")):
            if output.is_file():
                original = source / output.relative_to(destination)
                checksum = digest(original)
                if digest(output) != checksum:
                    raise ValueError(f"copied runtime data changed: {original}")
                DATA_FILES.append({"source": str(original),
                    "destination": str(output.relative_to(ROOT)), "sha256": checksum})


def wrapper(program, loader):
    """wrapper writes a fixed-path exec wrapper with private media library lookup."""
    output = ROOT / "usr/local/bin" / program
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text("#!/bin/sh\nset -eu\n"
                      "export OPENSSL_MODULES=\"${OPENSSL_MODULES:-/opt/oneapi-media/ossl-modules}\"\n"
                      "export OPENSSL_CONF=\"${OPENSSL_CONF:-/opt/oneapi-media/openssl.cnf}\"\n"
                      "export GCONV_PATH=\"${GCONV_PATH:-/opt/oneapi-media/gconv}\"\n"
                      f'exec /opt/oneapi-media/lib/{loader} --inhibit-cache '
                      '--library-path /opt/oneapi-media/lib '
                      f'/opt/oneapi-media/bin/{program} "$@"\n')
    output.chmod(0o755)


if __name__ == "__main__":
    ROOT.mkdir(exist_ok=False)
    initial = elf_metadata(Path("/usr/local/bin/ffmpeg"))
    MACHINE = initial["machine"]
    loader = Path(initial["interpreter"])
    copy_file(loader, LIB / loader.name, role="loader")
    for program in ("ffmpeg", "ffprobe"):
        path = Path("/usr/local/bin") / program
        if elf_metadata(path)["interpreter"] != str(loader):
            raise ValueError("tools use different ELF loaders")
        collect_elf(path, MEDIA / "bin" / program, role="program")
        wrapper(program, loader.name)
    for pattern, directory in (("/usr/lib/*/gconv", "gconv"),
                               ("/usr/lib/*/ossl-modules", "ossl-modules")):
        sources = [Path(path) for path in glob.glob(pattern)]
        if len(sources) != 1:
            raise ValueError(f"expected one target runtime module directory: {pattern}")
        source = sources[0]
        copy_tree(source, MEDIA / directory)
        for module in source.glob("*.so"):
            collect_elf(module, MEDIA / directory / module.name, role="module")
    for module in glob.glob("/usr/lib/*/libnss_*.so*"):
        collect_elf(Path(module), LIB / Path(module).name, role="module")
    copy_file(Path("/etc/ssl/openssl.cnf"), MEDIA / "openssl.cnf", role="config")
    copy_file(Path("/etc/nsswitch.conf"), ROOT / "etc/nsswitch.conf", role="config")
    copy_tree(Path("/usr/local/share"), ROOT / "usr/local/share")
    for directory in ("/usr/share/doc", "/usr/share/common-licenses"):
        copy_tree(Path(directory), MEDIA / "notices" / directory.lstrip("/"),
                  documentation=directory == "/usr/share/doc")
    for program in ("ffmpeg", "ffprobe"):
        result = subprocess.check_output([str(LIB / loader.name), "--inhibit-cache",
            "--library-path", str(LIB), "--list", str(MEDIA / "bin" / program)], text=True)
        outside = [line for line in result.splitlines() if "=> /" in line and str(LIB) not in line]
        if outside:
            raise ValueError(f"dependencies escaped private runtime: {outside}")
    (MEDIA / "manifest.json").write_text(json.dumps({
        "machine": MACHINE, "loader": loader.name, "files": RECORDS,
        "elf": ELF_FILES, "runtime_data": DATA_FILES,
        "source_dangling_doc_links": DANGLING_DOC_LINKS}, indent=2) + "\n")
    print(json.dumps({"machine": MACHINE, "files": len(RECORDS),
                      "copied_bytes": sum(path.stat().st_size for path in ROOT.rglob("*")
                                          if path.is_file())}))
