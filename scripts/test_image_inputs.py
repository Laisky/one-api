#!/usr/bin/env python3
"""Image input regressions use captured Docker calls and a moving synthetic registry."""
import json
from pathlib import Path
import re
import types
import unittest
import qualify_native_arm64 as native
import image_inputs as inputs
import copy

ROOT = Path(__file__).resolve().parents[1]
QUALIFIED = {
    "FFMPEG_IMAGE": "linuxserver/ffmpeg:latest@sha256:742e17516dd4d7b76b939d774bd08f0f6b7cab8bce1c014b95ab2fb992fd0a1f",
    "RUNTIME_IMAGE": "ubuntu:26.04@sha256:c098726dd9b9733f3e885f8de0ef375206a311b0a317eadc40d347b1115fb944",
    "NODE_IMAGE": "node:24-bookworm@sha256:6449e19d8184d84062525b6a7524def03146c2cb04a7f08b076680d0e8e4240a",
    "GO_IMAGE": "golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5",
}


class ImageInputTests(unittest.TestCase):
    """ImageInputTests prove delivery and qualification remain bound to the same immutable inputs."""

    def test_defaults_match_both_previously_qualified_index_sets(self):
        """test_defaults_match_both_previously_qualified_index_sets anchors the reviewed four-input migration."""
        self.assertEqual(inputs.read_image_inputs((ROOT/"Dockerfile").read_text(), "1.27.3"), QUALIFIED)
        self.assertEqual(inputs.REVIEWED_PIN_MIGRATION, QUALIFIED)

    def test_parser_rejects_unpinned_ambiguous_or_untracked_inputs(self):
        """test_parser_rejects_unpinned_ambiguous_or_untracked_inputs fails closed before a build."""
        source = (ROOT/"Dockerfile").read_text()
        for key, value in QUALIFIED.items():
            for change in (value.split("@")[0], value[:-1], value[:-64]+"A"*64,
                           "other/"+value, value.replace("1.27.3", "1.27.2") if key == "GO_IMAGE" else ""):
                with self.subTest(key=key, value=change), self.assertRaises(ValueError):
                    inputs.read_image_inputs(source.replace(value, change), "1.27.3")
        for change in (source.replace("ARG GO_IMAGE=", "ARG OTHER_IMAGE="),
                       source.replace("ARG NODE_IMAGE=", "ARG NODE_IMAGE\nARG NODE_IMAGE="),
                       source+"\nARG GO_IMAGE="+QUALIFIED["GO_IMAGE"]+"\n",
                       source.replace("FROM ${RUNTIME_IMAGE} AS final", "FROM ubuntu:26.04 AS final")):
            with self.subTest(source=change[-100:]), self.assertRaises(ValueError):
                inputs.read_image_inputs(change, "1.27.3")

    def test_docker_keyword_case_and_whitespace_cannot_hide_overrides(self):
        """test_docker_keyword_case_and_whitespace_cannot_hide_overrides checks Docker-valid spellings."""
        source = (ROOT/"Dockerfile").read_text()
        equivalent = source.replace("ARG ", "arg\t").replace("FROM ", "from  ")
        self.assertEqual(inputs.read_image_inputs(equivalent, "1.27.3"), QUALIFIED)
        for declaration in ("arg\tGO_IMAGE=", "ARG  GO_IMAGE=", "ARG GO_IMAGE ="):
            changed = source.replace("FROM ", declaration+QUALIFIED["GO_IMAGE"]+"\nFROM ", 1)
            with self.subTest(declaration=declaration), self.assertRaises(ValueError):
                inputs.read_image_inputs(changed, "1.27.3")
        for stage in ("FROM\tubuntu:26.04", "from ubuntu:26.04", "FROM"):
            with self.subTest(stage=stage), self.assertRaises(ValueError):
                inputs.read_image_inputs(source+"\n"+stage+"\n", "1.27.3")

    def test_indices_require_unique_native_children(self):
        """test_indices_require_unique_native_children rejects foreign, missing, duplicate and malformed descriptors."""
        children = [{"platform": {"os": "linux", "architecture": arch}, "digest": "sha256:"+char*64}
                    for arch, char in (("amd64", "a"), ("arm64", "b"))]
        manifest = {"manifests": children}
        self.assertEqual(inputs.native_platform_digest(manifest, "amd64"), "sha256:"+"a"*64)
        self.assertEqual(inputs.native_platform_digest(manifest, "arm64"), "sha256:"+"b"*64)
        invalid = [{}, {"manifests": []}, {"manifests": children[:1]}, {"manifests": children+[children[0]]}]
        for field, value in (("os", "windows"), ("architecture", "foreign")):
            changed = copy.deepcopy(manifest);changed["manifests"][1]["platform"][field] = value
            invalid.append(changed)
        changed = copy.deepcopy(manifest);changed["manifests"][0]["digest"] = "sha256:short";invalid.append(changed)
        for value in invalid:
            with self.subTest(manifest=value), self.assertRaises(RuntimeError):
                inputs.native_platform_digest(value, "arm64")
        with self.assertRaises(RuntimeError):
            inputs.native_platform_digest(manifest, "foreign")

    def test_builder_migration_does_not_hide_later_digest_or_instruction_drift(self):
        """test_builder_migration_does_not_hide_later_digest_or_instruction_drift limits the legacy exception."""
        legacy = ["ARG "+key+"="+value.split("@")[0] for key, value in QUALIFIED.items()]+["RUN fixture"]
        pinned = ["ARG "+key+"="+value for key, value in QUALIFIED.items()]+["RUN fixture"]
        self.assertEqual(set(inputs.builder_pin_migrations(pinned, legacy)), set(QUALIFIED))
        self.assertEqual(inputs.builder_pin_migrations(pinned, pinned), [])
        for index in range(4):
            wrong = list(pinned);wrong[index] = wrong[index][:-64]+"f"*64
            for baseline in (legacy, pinned):
                with self.subTest(index=index, baseline=baseline), self.assertRaises(RuntimeError):
                    inputs.builder_pin_migrations(wrong, baseline)
        for candidate in (pinned+["RUN unexpected"], pinned[:-1]+["RUN changed"],
                          [line.replace("node:24", "node:26") for line in pinned]):
            with self.subTest(candidate=candidate), self.assertRaises(RuntimeError):
                inputs.builder_pin_migrations(candidate, legacy)

    def test_registry_tag_movement_cannot_change_delivery_inputs(self):
        """test_registry_tag_movement_cannot_change_delivery_inputs exercises actual qualifier input calls."""
        defaults = dict(re.findall(r"(?m)^ARG ([A-Z_]+_IMAGE)=(\S+)$", (ROOT/"Dockerfile").read_text()))
        catalog = {value.split("@")[0]: value.split("@")[1] for value in QUALIFIED.values()}
        def run(argv, _name, *_args):
            """run returns a public dual-architecture fixture without executing Docker."""
            return json.dumps({"manifests": [
                {"platform": {"os": "linux", "architecture": arch}, "digest": "sha256:"+char*64}
                for arch, char in (("amd64", "a"), ("arm64", "b"))]})
        def identity(reference, _name):
            """identity resolves the qualification-time registry snapshot independently of later tag movement."""
            digest = reference.split("@")[-1] if "@" in reference else catalog[reference]
            return {"Id": "sha256:"+"c"*64, "Size": 1,
                    "RepoDigests": [reference.split(":")[0]+"@"+digest]}
        fake = types.SimpleNamespace(context=ROOT, run=run, identity=identity, architecture="amd64",
            platform="linux/amd64", summary={}, flush=lambda: None)
        native.Qualification.inputs(fake)
        qualified = {key: value.split("@")[1] for key, value in fake.pins.items()}
        for moved in (False, True):
            with self.subTest(tags_moved=moved):
                registry = {tag: "sha256:"+"f"*64 if moved else digest for tag, digest in catalog.items()}
                delivered = {key: value.split("@")[1] if "@" in value else registry[value]
                             for key, value in defaults.items()}
                self.assertEqual(delivered, qualified,
                                 "delivery defaults drifted from the inputs actually passed by qualification")


if __name__ == "__main__":
    unittest.main(verbosity=2)
