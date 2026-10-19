"""Verify OCI measurement identity and layer integrity against saved-image fixtures."""
import gzip
import hashlib
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

import measure_runtime_image as measurement


def saved_image(*, bad_diffid=False):
    """saved_image returns a Docker-save fixture with one independently hashed layer."""
    layer = b"synthetic layer payload" * 100000
    diffid = "sha256:" + hashlib.sha256(layer).hexdigest()
    config = json.dumps({"rootfs": {"type": "layers", "diff_ids": [
        "sha256:" + "0" * 64 if bad_diffid else diffid]}}).encode()
    config_id = "sha256:" + hashlib.sha256(config).hexdigest()
    manifest = json.dumps([{"Config": "config.json", "Layers": ["layer/layer.tar"]}]).encode()
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode="w") as archive:
        for name, data in [("config.json", config), ("layer/layer.tar", layer),
                           ("manifest.json", manifest)]:
            entry = tarfile.TarInfo(name)
            entry.size = len(data)
            archive.addfile(entry, io.BytesIO(data))
    raw.seek(0)
    return raw, config_id, layer


class FakeSave:
    """FakeSave supplies a finite docker-save stdout stream without Docker access."""

    def __init__(self, stream):
        """__init__ assigns the prepared image stream."""
        self.stdout = stream

    def wait(self, timeout=None):
        """wait reports a successful finite export."""
        return 0

    def poll(self):
        """poll reports that the fixture process already completed."""
        return 0


class ImageMeasurementTests(unittest.TestCase):
    """ImageMeasurementTests prevent size evidence from accepting corrupt OCI images."""

    def export(self, destination, *, bad_diffid=False, bad_identity=False):
        """export runs the collector against a specified saved-image control."""
        stream, image_id, layer = saved_image(bad_diffid=bad_diffid)
        identity = "sha256:" + "f" * 64 if bad_identity else image_id
        inspect = json.dumps([{"Id": identity, "Size": len(layer)}]).encode()
        with patch.object(measurement.subprocess, "Popen", return_value=FakeSave(stream)), \
                patch.object(measurement.subprocess, "check_output", return_value=inspect):
            return measurement.measure("fixture:image", destination), layer

    def test_roundtrip_preserves_blob_identity_and_measured_sizes(self):
        """test_roundtrip validates decompressible OCI blobs and exact byte accounting."""
        with tempfile.TemporaryDirectory() as parent:
            destination = Path(parent) / "oci"
            result, layer = self.export(destination)
            blob = destination / "blobs/sha256" / result["layers"][0]["digest"].split(":")[1]
            self.assertEqual(gzip.decompress(blob.read_bytes()), layer)
            self.assertEqual(blob.stat().st_size, result["compressed_layer_bytes"])
            self.assertEqual(hashlib.sha256(blob.read_bytes()).hexdigest(), blob.name)

    def test_mismatched_layer_is_rejected(self):
        """test_mismatched_layer rejects a config that does not identify saved layer bytes."""
        with tempfile.TemporaryDirectory() as parent:
            with self.assertRaisesRegex(ValueError, "uncompressed layers differ"):
                self.export(Path(parent) / "oci", bad_diffid=True)

    def test_mismatched_image_identity_is_rejected(self):
        """test_mismatched_image rejects evidence exported from another image."""
        with tempfile.TemporaryDirectory() as parent:
            with self.assertRaisesRegex(ValueError, "image identity changed"):
                self.export(Path(parent) / "oci", bad_identity=True)


if __name__ == "__main__":
    unittest.main(verbosity=2)

