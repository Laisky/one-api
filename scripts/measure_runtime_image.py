#!/usr/bin/env python3
"""Export a local image as an OCI layout with measured gzip-compressed layers.

This streams Docker's uncompressed save output without retaining a second large
archive. It never pushes or reads registry credentials. Compression uses gzip
level 6 with a fixed timestamp; compare baseline and candidate with this script.
Published registry compression may differ from this local OCI representation.
Schema version 2 counts uncompressed_bytes from selected layer tar streams,
including headers and padding. docker_inspect_size_bytes retains Docker's
store-dependent Size metric separately; historical reports are not rewritten.
"""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tarfile


def digest_file(path):
    """digest_file returns the SHA256 and length of a saved blob."""
    with path.open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
    return digest, path.stat().st_size


def store_bytes(blobs, value):
    """store_bytes writes JSON/config bytes into the content-addressed blob store."""
    digest = hashlib.sha256(value).hexdigest()
    (blobs / digest).write_bytes(value)
    return {"digest": "sha256:" + digest, "size": len(value)}


def measure(image, destination):
    """measure exports image layers and returns the exact local OCI descriptors."""
    destination.mkdir(parents=True, exist_ok=False)
    blobs = destination / "blobs" / "sha256"
    blobs.mkdir(parents=True)
    configs = {}
    compressed = {}
    diffids = {}
    uncompressed_sizes = {}
    process = subprocess.Popen(["docker", "image", "save", image], stdout=subprocess.PIPE)
    try:
        with tarfile.open(fileobj=process.stdout, mode="r|") as archive:
            for entry in archive:
                if not entry.isfile():
                    continue
                stream = archive.extractfile(entry)
                if entry.name.endswith(".json"):
                    if entry.size > 2 * 1024 * 1024:
                        raise ValueError("unexpectedly large image JSON")
                    configs[entry.name] = stream.read()
                elif entry.name.endswith("/layer.tar") or entry.name.startswith("blobs/"):
                    # Explicit layer paths are never metadata. Opaque blobs need
                    # complete bounded JSON validation, not a tar filename sniff.
                    if entry.name.startswith("blobs/") and entry.size <= 2 * 1024 * 1024:
                        candidate = stream.read()
                        try:
                            metadata = json.loads(candidate)
                        except (json.JSONDecodeError, UnicodeDecodeError):
                            metadata = None
                        if isinstance(metadata, dict):
                            configs[entry.name] = candidate
                            continue
                        stream = io.BytesIO(candidate)
                    temporary = blobs / "layer.pending"
                    uncompressed_digest = hashlib.sha256()
                    uncompressed_size = 0
                    with temporary.open("wb") as raw:
                        with gzip.GzipFile(filename="", mode="wb", fileobj=raw,
                                           compresslevel=6, mtime=0) as output:
                            while data := stream.read(1024 * 1024):
                                output.write(data)
                                uncompressed_digest.update(data)
                                uncompressed_size += len(data)
                    digest, size = digest_file(temporary)
                    temporary.rename(blobs / digest)
                    compressed[entry.name] = {
                        "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
                        "digest": "sha256:" + digest, "size": size,
                    }
                    diffids[entry.name] = "sha256:" + uncompressed_digest.hexdigest()
                    uncompressed_sizes[entry.name] = uncompressed_size
        if process.wait(timeout=120) != 0:
            raise RuntimeError("docker image save failed")
    finally:
        process.stdout.close()
        if process.poll() is None:
            process.terminate()
            process.wait(timeout=30)
    exported = json.loads(configs["manifest.json"])
    if len(exported) != 1:
        raise ValueError("select exactly one image")
    exported = exported[0]
    metadata = json.loads(configs[exported["Config"]])
    if [diffids[name] for name in exported["Layers"]] != metadata["rootfs"]["diff_ids"]:
        raise ValueError("exported uncompressed layers differ from image config")
    config = store_bytes(blobs, configs[exported["Config"]])
    config["mediaType"] = "application/vnd.oci.image.config.v1+json"
    layers = [compressed[name] for name in exported["Layers"]]
    manifest = {"schemaVersion": 2,
                "mediaType": "application/vnd.oci.image.manifest.v1+json",
                "config": config, "layers": layers}
    descriptor = store_bytes(blobs, json.dumps(manifest, separators=(",", ":")).encode())
    descriptor["mediaType"] = manifest["mediaType"]
    (destination / "index.json").write_text(json.dumps({
        "schemaVersion": 2, "manifests": [descriptor]}, indent=2))
    (destination / "oci-layout").write_text('{"imageLayoutVersion":"1.0.0"}\n')
    inspect = json.loads(subprocess.check_output(["docker", "image", "inspect", image]))[0]
    if config["digest"] != inspect["Id"]:
        raise ValueError("exported image identity changed")
    result = {"image": image, "id": inspect["Id"], "measurement_schema_version": 2,
              "uncompressed_bytes": sum(uncompressed_sizes[name] for name in exported["Layers"]),
              "uncompressed_bytes_definition": "selected layer tar streams including headers and padding",
              "docker_inspect_size_bytes": inspect["Size"],
              "compressed_layer_bytes": sum(layer["size"] for layer in layers),
              "compression": "gzip level 6, mtime 0; local OCI, not published",
              "manifest": descriptor, "layers": layers}
    (destination / "measurement.json").write_text(json.dumps(result, indent=2))
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("image")
    parser.add_argument("destination", type=Path)
    args = parser.parse_args()
    print(json.dumps(measure(args.image, args.destination), indent=2))
