"""Verify OCI measurement identity and tar integrity against saved-image fixtures."""
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

PAYLOAD = b"public synthetic layer payload\n" * 50


def layer_tar(name="ordinary.txt", payload=PAYLOAD):
    """layer_tar returns a deterministic valid layer with one known regular file."""
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode="w", format=tarfile.USTAR_FORMAT) as archive:
        entry = tarfile.TarInfo(name)
        entry.size = len(payload)
        entry.mode = 0o644
        entry.mtime = 0
        archive.addfile(entry, io.BytesIO(payload))
    return raw.getvalue()


def saved_image(*, layers=None, layout="legacy", bad_diffid=False, unreferenced=False):
    """saved_image returns Docker-save bytes with real tar layers and manifest last."""
    layers = [layer_tar()] if layers is None else layers
    diffids = ["sha256:" + hashlib.sha256(layer).hexdigest() for layer in layers]
    if bad_diffid:
        diffids[0] = "sha256:" + "0" * 64
    config = json.dumps({"rootfs": {"type": "layers", "diff_ids": diffids}}).encode()
    config_hash = hashlib.sha256(config).hexdigest()
    config_name = "config.json" if layout == "legacy" else "blobs/sha256/" + config_hash
    names = [(f"layer-{index}/layer.tar" if layout == "legacy" else
              "blobs/sha256/" + hashlib.sha256(layer).hexdigest())
             for index, layer in enumerate(layers)]
    manifest = json.dumps([{"Config": config_name, "Layers": names}]).encode()
    entries = [(config_name, config), *zip(names, layers)]
    if unreferenced:
        entries.append(("unused/layer.tar", layer_tar("unused.txt", b"not selected" * 400)))
    entries.append(("manifest.json", manifest))
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode="w") as archive:
        for name, data in entries:
            entry = tarfile.TarInfo(name)
            entry.size = len(data)
            archive.addfile(entry, io.BytesIO(data))
    raw.seek(0)
    return raw, "sha256:" + config_hash, layers


class FakeSave:
    """FakeSave supplies finite Docker-save stdout without Docker access."""

    def __init__(self, stream):
        """__init__ assigns the prepared saved-image stream."""
        self.stdout = stream

    def wait(self, timeout=None):
        """wait reports a successful finite export."""
        return 0

    def poll(self):
        """poll reports that the fixture process has completed."""
        return 0


class ImageMeasurementTests(unittest.TestCase):
    """ImageMeasurementTests reject corrupt identities and verify actual OCI tar media."""

    def export(self, destination, *, bad_identity=False, inspect_size=123456789, **kwargs):
        """export runs the collector with independently chosen image-size metadata."""
        stream, image_id, layers = saved_image(**kwargs)
        identity = "sha256:" + "f" * 64 if bad_identity else image_id
        inspect = json.dumps([{"Id": identity, "Size": inspect_size}]).encode()
        with patch.object(measurement.subprocess, "Popen", return_value=FakeSave(stream)), \
                patch.object(measurement.subprocess, "check_output", return_value=inspect):
            return measurement.measure("fixture:image", destination), layers

    def read_layer(self, destination, descriptor):
        """read_layer verifies a compressed descriptor and returns its valid tar bytes."""
        self.assertEqual(descriptor["mediaType"], "application/vnd.oci.image.layer.v1.tar+gzip")
        blob = destination / "blobs/sha256" / descriptor["digest"].split(":")[1]
        self.assertEqual(blob.stat().st_size, descriptor["size"])
        self.assertEqual(hashlib.sha256(blob.read_bytes()).hexdigest(), blob.name)
        layer = gzip.decompress(blob.read_bytes())
        with tarfile.open(fileobj=io.BytesIO(layer), mode="r:") as archive:
            members = archive.getmembers()
            self.assertEqual(len(members), 1)
            with archive.extractfile(members[0]) as member:
                return layer, members[0].name, member.read()

    def test_roundtrip_preserves_blob_identity_tar_contents_and_measured_sizes(self):
        """test_roundtrip verifies valid inner tar contents in both save layouts."""
        for layout in ("legacy", "blobs"):
            with self.subTest(layout=layout), tempfile.TemporaryDirectory() as parent:
                destination = Path(parent) / "oci"
                result, layers = self.export(destination, layout=layout)
                actual, name, payload = self.read_layer(destination, result["layers"][0])
                self.assertEqual(actual, layers[0])
                self.assertEqual((name, payload), ("ordinary.txt", PAYLOAD))
                self.assertEqual(result["compressed_layer_bytes"], result["layers"][0]["size"])

    def test_leading_brace_tar_filename_is_never_json_metadata(self):
        """test_leading_brace preserves small tar layers resembling JSON prefixes."""
        layer = layer_tar("{unusual}.txt")
        self.assertTrue(layer.startswith(b"{"))
        for layout in ("legacy", "blobs"):
            with self.subTest(layout=layout), tempfile.TemporaryDirectory() as parent:
                destination = Path(parent) / "oci"
                result, _ = self.export(destination, layout=layout, layers=[layer])
                actual, name, payload = self.read_layer(destination, result["layers"][0])
                self.assertEqual((actual, name, payload), (layer, "{unusual}.txt", PAYLOAD))

    def test_uncompressed_counter_uses_selected_streams_not_inspect_size(self):
        """test_uncompressed_counter excludes unused blobs and preserves inspect data separately."""
        layers = [layer_tar(), layer_tar("second.txt", b"second body" * 3000)]
        for inspect_size in (7, 123456789):
            with self.subTest(inspect_size=inspect_size), tempfile.TemporaryDirectory() as parent:
                destination = Path(parent) / "oci"
                result, _ = self.export(destination, layers=layers, unreferenced=True,
                                        inspect_size=inspect_size)
                self.assertEqual(result["measurement_schema_version"], 2)
                self.assertEqual(result["uncompressed_bytes"], sum(map(len, layers)))
                self.assertEqual(result["docker_inspect_size_bytes"], inspect_size)
                self.assertEqual(len(result["layers"]), 2)
                self.assertEqual(result["compressed_layer_bytes"], sum(d["size"] for d in result["layers"]))

    def test_large_layer_stream_roundtrip_is_valid_tar(self):
        """test_large_layer exercises streaming beyond the bounded JSON candidate size."""
        payload = b"streamed payload" * 180000
        layer = layer_tar("large.txt", payload)
        self.assertGreater(len(layer), 2 * 1024 * 1024)
        with tempfile.TemporaryDirectory() as parent:
            destination = Path(parent) / "oci"
            result, _ = self.export(destination, layers=[layer], layout="blobs")
            actual, name, body = self.read_layer(destination, result["layers"][0])
            self.assertEqual((actual, name, body), (layer, "large.txt", payload))
            self.assertEqual(result["uncompressed_bytes"], len(layer))

    def test_mismatched_layer_is_rejected(self):
        """test_mismatched_layer rejects a config that identifies different layer bytes."""
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
