#!/usr/bin/env python3
"""Canonical Dockerfile image inputs and the reviewed initial digest migration."""
import re
import shlex

# These are the four shared index digests independently qualified at 076e5335.
# This allowlist permits only the original floating declaration to these exact pins.
# Pinned-to-pinned changes remain visible to the normal builder comparison.
REVIEWED_PIN_MIGRATION = {'NODE_IMAGE': 'node:24-bookworm@sha256:6449e19d8184d84062525b6a7524def03146c2cb04a7f08b076680d0e8e4240a', 'GO_IMAGE': 'golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5', 'FFMPEG_IMAGE': 'linuxserver/ffmpeg:latest@sha256:742e17516dd4d7b76b939d774bd08f0f6b7cab8bce1c014b95ab2fb992fd0a1f', 'RUNTIME_IMAGE': 'ubuntu:26.04@sha256:c098726dd9b9733f3e885f8de0ef375206a311b0a317eadc40d347b1115fb944'}


def read_image_inputs(source, go_version):
    """read_image_inputs requires four unique global immutable defaults and only declared image stages."""
    tags = {"NODE_IMAGE": "node:24-bookworm", "GO_IMAGE": "golang:"+go_version+"-bookworm",
            "FFMPEG_IMAGE": "linuxserver/ffmpeg:latest", "RUNTIME_IMAGE": "ubuntu:26.04"}
    inputs, stages, started = {}, set(), False
    for line in source.splitlines():
        text = line.strip()
        if re.match(r"FROM(?:\s|$)", text, re.I):
            started = True
            words = shlex.split(text)
            offset = 2 if len(words) > 1 and words[1].startswith("--platform=") else 1
            if len(words) <= offset:
                raise ValueError("missing FROM image")
            reference = words[offset]
            if reference not in {"${"+key+"}" for key in tags} | stages:
                raise ValueError("FROM must use a declared pinned image or an earlier stage")
            if len(words) > offset+1:
                if len(words) != offset+3 or words[offset+1].upper() != "AS":
                    raise ValueError("unsupported FROM declaration")
                stages.add(words[offset+2])
        declaration = re.match(r"ARG\s+([A-Za-z_][A-Za-z0-9_]*)", text, re.I)
        if not declaration:
            continue
        match = re.fullmatch(r"ARG\s+([A-Za-z_][A-Za-z0-9_]*)(?:=(\S+))?", text, re.I)
        if not match:
            if declaration[1] in tags or declaration[1].endswith("_IMAGE"):
                raise ValueError("unsupported image ARG declaration")
            continue
        key, value = match.groups()
        if key.endswith("_IMAGE") and key not in tags:
            raise ValueError("unexpected image input: "+key)
        if key not in tags:
            continue
        if started or key in inputs:
            raise ValueError("image defaults must be unique global ARG declarations")
        if not value or not re.fullmatch(re.escape(tags[key])+r"@sha256:[0-9a-f]{64}", value):
            raise ValueError("image input must retain its reviewed repository/tag and an immutable digest: "+key)
        inputs[key] = value
    if set(inputs) != set(tags):
        raise ValueError("all four pinned image defaults are required")
    return inputs


def native_platform_digest(manifest, architecture):
    """native_platform_digest requires one Linux descriptor for each supported native architecture."""
    selected = {}
    for arch in ("amd64", "arm64"):
        matches = [item for item in manifest.get("manifests", [])
                   if item.get("platform", {}).get("os") == "linux"
                   and item["platform"].get("architecture") == arch]
        if len(matches) != 1 or not re.fullmatch(r"sha256:[0-9a-f]{64}", matches[0].get("digest", "")):
            raise RuntimeError("input index must uniquely resolve Linux "+arch)
        selected[arch] = matches[0]["digest"]
    if architecture not in selected:
        raise RuntimeError("unsupported native input architecture")
    return selected[architecture]


def builder_pin_migrations(candidate, baseline):
    """builder_pin_migrations permits only reviewed floating-to-digest declarations and preserves other drift."""
    expected, migrated = list(baseline), []
    for key, pin in REVIEWED_PIN_MIGRATION.items():
        old, new = "ARG "+key+"="+pin.split("@")[0], "ARG "+key+"="+pin
        if old in expected and new in candidate:
            expected[expected.index(old)] = new
            migrated.append(key)
    if candidate != expected:
        raise RuntimeError("production frontend or Go builder recipe differs from approved baseline")
    return migrated
