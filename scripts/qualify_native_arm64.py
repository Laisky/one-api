#!/usr/bin/env python3
"""Bounded actual-image native qualification; only compact receipts enter evidence."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import urllib.request
import uuid

FFMPEG = "linuxserver/ffmpeg@sha256:742e17516dd4d7b76b939d774bd08f0f6b7cab8bce1c014b95ab2fb992fd0a1f"
UBUNTU = "ubuntu@sha256:c098726dd9b9733f3e885f8de0ef375206a311b0a317eadc40d347b1115fb944"
ORIGINAL = "f4f26683404611c2313594e98e70ff5b36647dfd"
OWNERSHIP = "ec72e68fc2c30363402e974d267674339159f0c2"
# Historical runtime controls remain immutable; production includes merged security fixes.
PRODUCTION_BASELINE = "d316af5675f1363ec8d06ed3e92c15ed0d9fa0a4"
GO_VERSION = "1.27.3"
GIB, LIMIT = 1024**3, 4*1024**2
TOKENS = {
    "cl100k_base": (1681126, "223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7"),
    "o200k_base": (3613922, "446a9538cb6c348e3516120d7c08b09f57c36495e2acfffe59a5bf8b0cfb1a2d"),
    "p50k_base": (836186, "94b5ca7dff4d00767bc256fdd1b27e5b17361d7b8a5f968547f9f23eb70d2069"),
    "r50k_base": (835554, "306cd27f03c1a714eca7108e03d66b7dc042abe8c258b44c199a7ed9838dd930"),
}


def save(path, value):
    """save writes a bounded atomic JSON receipt."""
    data = json.dumps(value, indent=2, sort_keys=True)+"\n"
    if len(data.encode()) > LIMIT:
        raise RuntimeError("receipt exceeds artifact limit")
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(".tmp")
    temporary.write_text(data)
    temporary.replace(path)


def sha(path):
    """sha hashes files with bounded memory."""
    result = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1048576), b""):
            result.update(block)
    return result.hexdigest()


def elf_machine(header):
    """elf_machine validates ELF64 independently of the collector manifest."""
    if len(header) < 20 or header[:5] != b"\x7fELF\x02" or header[5] not in (1, 2):
        raise ValueError("expected ELF64")
    return int.from_bytes(header[18:20], "little" if header[5] == 1 else "big")


def architecture_profile(architecture):
    """architecture_profile returns strict native identity controls for one explicit target."""
    profiles = {
        "arm64": {"aliases": ("aarch64", "arm64"), "machine": 183, "loader": "ld-linux-aarch64.so.1"},
        "amd64": {"aliases": ("x86_64", "amd64"), "machine": 62, "loader": "ld-linux-x86-64.so.2"},
    }
    if architecture not in profiles:
        raise ValueError("architecture must be explicitly arm64 or amd64")
    return profiles[architecture]


def assert_native(machine, docker_arch, go, architecture="arm64"):
    """assert_native refuses foreign hosts, Docker daemons and Go compilers for the selected target."""
    profile = architecture_profile(architecture)
    if machine.lower() not in profile["aliases"] or docker_arch.lower() not in profile["aliases"]:
        raise RuntimeError("native "+architecture+" host and Docker server required")
    expected = {"GOARCH": architecture, "GOHOSTARCH": architecture, "GOOS": "linux",
                "GOHOSTOS": "linux", "GOVERSION": "go"+GO_VERSION}
    if any(go.get(key) != value for key, value in expected.items()):
        raise RuntimeError("native Linux "+architecture+" Go "+GO_VERSION+" required")


def assert_native_elf(header, architecture):
    """assert_native_elf independently binds actual ELF bytes to the explicit native target."""
    if elf_machine(header) != architecture_profile(architecture)["machine"]:
        raise RuntimeError("actual ELF machine is not "+architecture)


def assert_manifest_architecture(manifest, architecture):
    """assert_manifest_architecture binds donor metadata and private loader to the native target."""
    profile = architecture_profile(architecture)
    if manifest.get("machine") != profile["machine"] or manifest.get("loader") != profile["loader"]:
        raise RuntimeError("collector machine or loader does not match "+architecture)


def assert_application_compiler(metadata):
    """assert_application_compiler verifies extracted ELF build information without executing it."""
    if not re.match(r"^[^\n]+: go"+re.escape(GO_VERSION)+r"\n", metadata):
        raise RuntimeError("actual application was not built with Go "+GO_VERSION)


def runtime_recipe(source):
    """runtime_recipe substitutes only the historical builder artifact COPY."""
    marker = "FROM ${FFMPEG_IMAGE} AS final"
    if source.count(marker) != 1:
        raise ValueError("historical runtime stage must be unique")
    runtime = source[source.index(marker):]
    pattern = r"COPY --from=go-builder( --chown=\${ONEAPI_UID}:\${ONEAPI_GID})? /out/one-api /one-api"
    matches = list(re.finditer(pattern, runtime))
    if len(matches) != 1:
        raise ValueError("historical application COPY must be unique")
    match = matches[0]
    runtime = runtime[:match.start()]+"COPY"+(match.group(1) or "")+" one-api /one-api"+runtime[match.end():]
    return "# syntax=docker/dockerfile:1.7\nARG FFMPEG_IMAGE\n"+runtime


def normalized_recipe(source):
    """normalized_recipe isolates the two approved ownership layer differences."""
    runtime = runtime_recipe(source).replace("--chown=${ONEAPI_UID}:${ONEAPI_GID} ", "")
    runtime = runtime.replace(" --chmod=755", "")
    runtime = runtime.replace("chown oneapi:oneapi /one-api /data", "chown oneapi:oneapi /data")
    runtime = runtime.replace("COPY one-api /one-api\n", "")
    runtime = re.sub(r"^RUN chown oneapi:oneapi /one-api\s*$", "", runtime, flags=re.M)
    runtime = re.sub(r"^RUN chmod \+x /usr/local/bin/docker-entrypoint.sh\s*$", "", runtime, flags=re.M)
    return [line.strip() for line in runtime.splitlines() if line.strip()
            and not line.lstrip().startswith(("#", "ARG ", "EXPOSE "))]


def builder_recipe(source):
    """builder_recipe returns production builder instructions for strict baseline comparison."""
    marker = "FROM ${FFMPEG_IMAGE}"
    if source.count(marker) != 1:
        raise ValueError("expected one FFmpeg stage boundary")
    return [line for line in source.split(marker)[0].splitlines() if line.strip()
            and not line.lstrip().startswith("#") and not line.startswith("ARG RUNTIME_IMAGE=")]


def runtime_records(manifest):
    """runtime_records validates relative collector destinations before forming absolute paths."""
    records = [dict(item) for item in manifest["files"]+manifest.get("runtime_data", [])]
    for item in records:
        destination = item["destination"]
        if (destination.startswith("/") or ".." in Path(destination).parts
                or str(Path(destination)) != destination
                or not destination.startswith(("opt/oneapi-media/", "usr/local/share/"))
                or not re.fullmatch(r"[A-Za-z0-9_./+@=-]+", destination)):
            raise RuntimeError("retained destination must be a safe confined relative path")
        item["destination"] = "/"+destination
    return records


class Qualification:
    """Qualification supervises only its own process groups and labelled containers."""

    def __init__(self, args):
        """__init__ separates executable scratch from uploadable evidence."""
        self.args, self.repo = args, Path(__file__).resolve().parents[1]
        self.architecture = getattr(args, "architecture", "arm64")
        self.profile = architecture_profile(self.architecture)
        self.platform = "linux/"+self.architecture
        self.evidence = args.evidence.resolve()
        self.evidence.mkdir(parents=True, exist_ok=True)
        if any(self.evidence.iterdir()):
            raise ValueError("evidence must initially be empty")
        self.work = Path(tempfile.mkdtemp(prefix="oneapi-native-"+self.architecture+"-"))
        self.token, self.started = uuid.uuid4().hex[:12], time.monotonic()
        self.docker, self.disks = shutil.which("docker"), [self.work, self.evidence]
        self.containers_enabled = False
        self.env = dict(os.environ, GOMAXPROCS="2", GOFLAGS="-p=2", TMPDIR=str(self.work),
                        GOCACHE=str(self.work/"go-cache"), GOMODCACHE=str(self.work/"go-modules"))
        self.summary = {"state": "incomplete", "owner": self.token, "scratch": str(self.work),
                        "commands": [], "images": {}, "checks": {}, "architecture": self.architecture,
                        "platform": self.platform, "expected_elf_machine": self.profile["machine"]}
        self.flush()

    def flush(self):
        """flush preserves partial evidence on every transition."""
        save(self.evidence/"summary.json", self.summary)

    def resources(self):
        """resources records disk, available memory, CPU and cgroup ceilings."""
        memory = {}
        for line in Path("/proc/meminfo").read_text().splitlines():
            key, value = line.split(":", 1)
            if key in ("MemTotal", "MemAvailable"):
                memory[key] = int(value.split()[0])*1024
        disks = {}
        for path in self.disks:
            usage = shutil.disk_usage(path)
            disks[str(path)] = {"free": usage.free, "total": usage.total}
        groups = {}
        for name in ("memory.max", "memory.current", "cpu.max"):
            path = Path("/sys/fs/cgroup")/name
            if path.is_file():
                groups[name] = path.read_text().strip()
        return {"memory": memory, "disks": disks, "cpu_count": os.cpu_count(),
                "affinity": len(os.sched_getaffinity(0)), "cgroups": groups}

    def guard(self):
        """guard aborts at resource floors or the overall deadline."""
        state = self.resources()
        if time.monotonic()-self.started > self.args.timeout:
            raise RuntimeError("total qualification deadline exceeded")
        if min(item["free"] for item in state["disks"].values()) < 3*GIB:
            raise RuntimeError("qualification aborted at 3 GiB disk floor")
        if state["memory"]["MemAvailable"] < GIB:
            raise RuntimeError("qualification aborted below 1 GiB available memory")

    def run(self, argv, name, timeout=300, env=None, cwd=None, check=True):
        """run drains bounded logs and supervises only the new process group."""
        self.guard()
        log = self.evidence/f"{len(self.summary['commands']):03d}-{name}.log"
        record = {"argv": list(map(str, argv)), "log": log.name, "state": "running"}
        self.summary["commands"].append(record)
        self.flush()
        start = time.monotonic()
        process = subprocess.Popen(argv, cwd=cwd or self.repo, env=env or self.env,
                                   stdout=subprocess.PIPE, stderr=subprocess.STDOUT, start_new_session=True)
        record["process_group"] = process.pid
        data, count = bytearray(), [0]
        def drain():
            """drain consumes all output but stores at most four MiB."""
            with log.open("wb") as output:
                for block in iter(lambda: process.stdout.read(65536), b""):
                    count[0] += len(block)
                    output.write(block[:max(0, LIMIT-output.tell())])
                    data.extend(block[:max(0, LIMIT-len(data))])
        reader = threading.Thread(target=drain, daemon=True)
        reader.start()
        try:
            while process.poll() is None or reader.is_alive():
                self.guard()
                if time.monotonic()-start > timeout:
                    raise RuntimeError(name+" command deadline exceeded")
                time.sleep(0.5)
            reader.join(timeout=10)
            record.update(returncode=process.returncode, state="passed" if not process.returncode else "failed",
                          seconds=time.monotonic()-start, bytes=count[0], truncated=count[0] > LIMIT)
            if check and process.returncode:
                raise RuntimeError(f"{name} failed ({process.returncode}); see {log.name}")
            return data.decode(errors="replace")
        except BaseException as error:
            # A finished parent can leave descendants holding its output pipe open.
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                pass
            reader.join(timeout=2)
            if process.poll() is None or reader.is_alive():
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait(timeout=5)
                reader.join(timeout=5)
            record.update(state="failed", error=str(error), seconds=time.monotonic()-start)
            raise
        finally:
            if not reader.is_alive():
                process.stdout.close()
            self.flush()

    def setup(self):
        """setup rejects emulated hosts and scopes descendant Docker container ownership."""
        if platform.system() != "Linux" or platform.machine().lower() not in self.profile["aliases"]:
            raise RuntimeError("native Linux "+self.architecture+" host required; refusing to build")
        if not self.docker:
            raise RuntimeError("Docker CLI unavailable")
        info = json.loads(self.run([self.docker, "info", "--format", "{{json .}}"], "docker-info"))
        go = json.loads(self.run(["go", "env", "-json", "GOARCH", "GOHOSTARCH", "GOOS",
                                 "GOHOSTOS", "GOVERSION"], "native-go"))
        assert_native(platform.machine(), info["Architecture"], go, self.architecture)
        self.disks.append(Path(info["DockerRootDir"]))
        self.summary["initial_resources"] = self.resources()
        self.summary["host"] = {"machine": platform.machine(), "docker_arch": info["Architecture"],
                               "docker_cpus": info["NCPU"], "docker_memory": info["MemTotal"], "go": go}
        if min(value["free"] for value in self.resources()["disks"].values()) < 12*GIB:
            raise RuntimeError("at least 12 GiB initially free on each relevant filesystem required")
        builder = self.run([self.docker, "buildx", "inspect", "default"], "default-builder")
        if not re.search(r"^Driver:\s+docker\s*$", builder, re.M):
            raise RuntimeError("existing default Docker driver required; no builder will be created")
        folder = self.work/"command-bin"
        folder.mkdir()
        shim = ("#!/usr/bin/env python3\nimport os,sys\na=sys.argv[1:]\n"
                "if a and a[0] in ('run','create'):\n"
                f" a[1:1]=['--label','oneapi.native.owner={self.token}']\n"
                f"os.execv({self.docker!r},[{self.docker!r},*a])\n")
        (folder/"docker").write_text(shim)
        (folder/"docker").chmod(0o755)
        self.env["PATH"] = str(folder)+os.pathsep+self.env["PATH"]
        self.containers_enabled = True
        self.flush()

    def source(self):
        """source validates exact head, production compatibility and tracked build context."""
        head = self.run(["git", "rev-parse", "HEAD"], "source-head").strip()
        if head != self.args.expected_head:
            raise RuntimeError("checkout differs from requested head")
        if self.run(["git", "status", "--porcelain", "--untracked-files=normal"], "source-status").strip():
            raise RuntimeError("qualification requires a clean exact-head checkout")
        self.run(["git", "merge-base", "--is-ancestor", PRODUCTION_BASELINE, head], "production-ancestry")
        names = self.run(["git", "diff", "--no-renames", "--name-only", "-z", PRODUCTION_BASELINE, head],
                         "production-diff").split("\0")
        changed = [name for name in names if name in ("go.mod", "go.sum", ".dockerignore", "scripts/docker-entrypoint.sh",
                                                    "model/dashboard_aggregate.sql")
                   or name.startswith("web/") or (name.endswith(".go") and not name.endswith("_test.go"))]
        if changed:
            raise RuntimeError("production application differs from approved baseline: "+repr(changed))
        baseline = self.run(["git", "show", PRODUCTION_BASELINE+":Dockerfile"], "builder-baseline")
        if builder_recipe((self.repo/"Dockerfile").read_text()) != builder_recipe(baseline):
            raise RuntimeError("production frontend or Go builder recipe differs from approved baseline")
        self.summary["checks"]["production_builder_recipe"] = "identical to approved production baseline"
        self.summary["checks"]["production_baseline"] = PRODUCTION_BASELINE
        paths = self.run(["git", "ls-files", "-z"], "source-files").split("\0")
        save(self.evidence/"source-receipt.json", {"head": head,
             "tree": self.run(["git", "rev-parse", "HEAD^{tree}"], "source-tree").strip(),
             "files": {name: sha(self.repo/name) for name in paths if name and (self.repo/name).is_file()},
             "production_baseline": PRODUCTION_BASELINE, "production_changes_since_baseline": changed})
        archive = self.work/"source.tar"
        self.run(["git", "archive", "--format=tar", "-o", str(archive), head], "source-archive")
        self.context = self.work/"source"
        self.context.mkdir()
        with tarfile.open(archive) as source:
            source.extractall(self.context, filter="data")
        archive.unlink()
        self.entrypoint_hash = sha(self.context/"scripts/docker-entrypoint.sh")

    def identity(self, image, name):
        """identity independently verifies actual image architecture and records bytes."""
        item = json.loads(self.run(["docker", "image", "inspect", image], name))[0]
        if (item["Os"], item["Architecture"]) != ("linux", self.architecture):
            raise RuntimeError("actual image is not "+self.platform+": "+image)
        self.summary["images"][image] = {key: item.get(key) for key in
                                        ("Id", "Size", "Os", "Architecture", "RepoDigests")}
        self.flush()
        return item

    def inputs(self):
        """inputs pins moving builder tags before building and records platform digests."""
        self.pins = {}
        for name, reference in (("FFMPEG_IMAGE", FFMPEG), ("RUNTIME_IMAGE", UBUNTU),
                                ("NODE_IMAGE", "node:24-bookworm"), ("GO_IMAGE", "golang:"+GO_VERSION+"-bookworm")):
            self.run(["docker", "pull", "--platform", self.platform, reference], "pull-"+name, 900)
            item = self.identity(reference, "inspect-"+name)
            pin = reference if "@sha256:" in reference else next(value for value in item["RepoDigests"]
                                                               if "@sha256:" in value)
            self.pins[name] = pin
            raw = json.loads(self.run(["docker", "buildx", "imagetools", "inspect", "--raw", pin],
                                      "manifest-"+name))
            matches = [value for value in raw.get("manifests", []) if value.get("platform", {}).get("os") == "linux"
                       and value["platform"].get("architecture") == self.architecture]
            if "manifests" in raw and len(matches) != 1:
                raise RuntimeError("input does not uniquely resolve "+self.platform)
            self.summary.setdefault("inputs", {})[name] = {"pin": pin, "image_id": item["Id"], "size": item["Size"],
                "platform_digest": matches[0]["digest"] if matches else pin.split("@")[-1]}
        self.flush()

    def build(self, name, context, custom=False):
        """build uses the default native builder and loads only owned local tags."""
        tag = f"oneapi-native-{self.architecture}-{self.token}:{name}"
        argv = ["docker", "buildx", "build", "--builder", "default", "--platform", self.platform,
                "--load", "--label", "oneapi.native.owner="+self.token, "-t", tag]
        for key, value in self.pins.items():
            argv += ["--build-arg", key+"="+value]
        if custom:
            argv += ["--build-arg", "ONEAPI_UID=12001", "--build-arg", "ONEAPI_GID=12002"]
        self.run([*argv, str(context)], "build-"+name, 3600)
        self.identity(tag, "image-"+name)
        return tag

    def extract(self, image, files, destination):
        """extract copies explicit artifacts from a never-started container."""
        name = "oneapi-native-extract-"+self.token+"-"+uuid.uuid4().hex[:6]
        self.run(["docker", "create", "--name", name, "--network", "none",
                  "--entrypoint", "/bin/true", image], "extract-create")
        try:
            for source, target in files.items():
                output = destination/target
                output.parent.mkdir(parents=True, exist_ok=True)
                self.run(["docker", "cp", name+":"+source, str(output)], "extract-copy")
        finally:
            self.run(["docker", "rm", name], "extract-remove", check=False)

    def controls(self, media):
        """controls reconstructs original and ownership runtimes with the same actual app."""
        artifact = self.work/"application"
        artifact.mkdir()
        self.extract(media, {"/one-api": "one-api", "/usr/local/bin/docker-entrypoint.sh": "entrypoint.sh"}, artifact)
        with (artifact/"one-api").open("rb") as source:
            assert_native_elf(source.read(20), self.architecture)
        if sha(artifact/"entrypoint.sh") != self.entrypoint_hash:
            raise RuntimeError("candidate entrypoint differs from unchanged source")
        self.app_hash = sha(artifact/"one-api")
        metadata = self.run(["go", "version", "-m", str(artifact/"one-api")], "application-build-info")
        assert_application_compiler(metadata)
        self.summary["checks"]["application_go_version"] = GO_VERSION
        original = self.run(["git", "show", ORIGINAL+":Dockerfile"], "original-dockerfile")
        ownership = self.run(["git", "show", OWNERSHIP+":Dockerfile"], "ownership-dockerfile")
        if normalized_recipe(original) != normalized_recipe(ownership):
            raise RuntimeError("control delta exceeds approved ownership and chmod operations")
        images = {}
        for name, text in (("original", original), ("ownership", ownership)):
            context = self.work/name
            (context/"scripts").mkdir(parents=True)
            shutil.copy2(artifact/"one-api", context/"one-api")
            shutil.copy2(self.context/"scripts/docker-entrypoint.sh", context/"scripts/docker-entrypoint.sh")
            (context/"Dockerfile").write_text(runtime_recipe(text))
            save(self.evidence/(name+"-recipe.json"), {"runtime_recipe": runtime_recipe(text),
                 "source": ORIGINAL if name == "original" else OWNERSHIP, "app_sha256": self.app_hash,
                 "entrypoint_sha256": self.entrypoint_hash,
                 "allowed_delta": ["COPY --chown replaces RUN chown", "COPY --chmod replaces RUN chmod"]})
            images[name] = self.build(name, context)
            if name == "ownership":
                images["ownership-custom"] = self.build("ownership-custom", context, True)
        return images

    def tokenizer(self):
        """tokenizer prepares four verified official public encoding files outside evidence."""
        cache = self.work/"tokenizer"
        cache.mkdir()
        receipt = {}
        for name, (size, expected) in TOKENS.items():
            url = "https://openaipublic.blob.core.windows.net/encodings/"+name+".tiktoken"
            path = cache/hashlib.sha1(url.encode()).hexdigest()
            with urllib.request.urlopen(url, timeout=60) as response:
                data = response.read(size+1)
            if len(data) != size or hashlib.sha256(data).hexdigest() != expected:
                raise RuntimeError("public tokenizer hash/size mismatch: "+name)
            path.write_bytes(data)
            receipt[name] = {"url": url, "size": size, "sha256": expected, "cache_name": path.name}
            self.guard()
        save(self.evidence/"tokenizer-receipt.json", receipt)
        return cache

    def shell(self, image, command, name):
        """shell explicitly invokes utilities with no app entrypoint or external network."""
        return self.run(["docker", "run", "--rm", "--network", "none", "--read-only",
                         "--user", "10001:10001", "--memory", "256m", "--cpus", "1",
                         "--pids-limit", "64", "--entrypoint", "/bin/sh", image, "-c", command], name)

    def runtime(self, image, cache, custom):
        """runtime requires identical app bytes and all four entrypoint/SQLite/UID checks."""
        text = self.shell(image, "sha256sum /one-api /usr/local/bin/docker-entrypoint.sh; "
                          "stat -c '%u:%g %a' /one-api; od -An -v -t x1 -N20 /one-api", "app-identity")
        lines = text.splitlines()
        if lines[0].split()[0] != self.app_hash or lines[1].split()[0] != self.entrypoint_hash:
            raise RuntimeError("actual image app or entrypoint bytes differ")
        if lines[2] != ("12001:12002 755" if custom else "10001:10001 755"):
            raise RuntimeError("actual app ownership/mode differs")
        assert_native_elf(bytes.fromhex("".join(lines[3:])), self.architecture)
        env = dict(self.env, ONEAPI_RUNTIME_TEST_IMAGE=image, ONEAPI_RUNTIME_TIKTOKEN_CACHE=str(cache),
                   ONEAPI_RUNTIME_TEST_UID="12001" if custom else "10001",
                   ONEAPI_RUNTIME_TEST_GID="12002" if custom else "10001")
        output = self.run([sys.executable, "scripts/test_runtime_image.py"], "runtime-"+image.rsplit(":", 1)[-1],
                          900, env=env, cwd=self.context)
        if "Ran 4 tests" not in output or re.search(r"\bskipped\b", output, re.I) or "\nOK\n" not in output:
            raise RuntimeError("runtime suite did not pass all four tests without skips")
        self.summary["checks"].setdefault("runtime", {})[image] = {"state": "passed", "tests": 4,
                                                                  "app_sha256": self.app_hash}

    def copy_reports(self, root):
        """copy_reports allowlists compact reports while excluding corpus and executables."""
        total = 0
        for path in sorted(root.rglob("*")):
            relative = path.relative_to(root)
            if not path.is_file() or path.suffix not in (".json", ".log"):
                continue
            if "corpus" in relative.parts:
                continue
            if "test-binaries" in relative.parts and not (
                    path.name == "receipt.json" or path.name.endswith("-build.log")):
                continue
            size = path.stat().st_size
            if size > LIMIT or total+size > 32*1024**2:
                raise RuntimeError("media reports exceed artifact allowance")
            target = self.evidence/"media-reports"/relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(path, target)
            total += size

    def media(self, images, cache):
        """media reuses the same generated corpus and native Go tests for both controls."""
        root = self.work/"media-suite"
        try:
            for index, candidate in enumerate((images["ownership"], images["media"])):
                argv = [sys.executable, "scripts/qualify_media_runtime.py", "--baseline", images["original"],
                        "--candidate", candidate, "--evidence", str(root), "--tokenizer-cache", str(cache)]
                if index:
                    argv.append("--reuse")
                self.run(argv, "media-ownership" if index == 0 else "media-closure", 2700, cwd=self.context)
                latest = max(root.glob("run-*/summary.json"), key=lambda item: item.stat().st_mtime_ns)
                receipt = json.loads(latest.read_text())
                if receipt["state"] != "passed" or receipt["fixture_count"] != 23:
                    raise RuntimeError("actual media comparison did not pass all 23 fixtures")
                self.summary["checks"].setdefault("media", {})[candidate] = receipt
        finally:
            if root.exists():
                self.copy_reports(root)

    def integrity(self, image):
        """integrity verifies complete retained hashes, native ELF bytes and pinned donor tools."""
        root = self.work/"integrity"
        root.mkdir()
        self.extract(image, {"/opt/oneapi-media/manifest.json": "manifest.json"}, root)
        manifest = json.loads((root/"manifest.json").read_text())
        assert_manifest_architecture(manifest, self.architecture)
        records = runtime_records(manifest)
        for offset in range(0, len(records), 80):
            batch = records[offset:offset+80]
            output = self.shell(image, "sha256sum "+" ".join(item["destination"] for item in batch), "retained-hashes")
            actual = {line.split(maxsplit=1)[1].strip(): line.split()[0] for line in output.splitlines()}
            if any(actual.get(item["destination"]) != item["sha256"] for item in batch):
                raise RuntimeError("actual retained hash mismatch")
        elfs = [item for item in records if item.get("role") in ("program", "loader", "library", "module")]
        for offset in range(0, len(elfs), 80):
            batch = elfs[offset:offset+80]
            command = "for f in "+" ".join(item["destination"] for item in batch)+"; do "
            command += """printf '%s ' "$f"; od -An -v -t x1 -N20 "$f" | tr -d ' \n'; printf '\n'; done"""
            output = self.shell(image, command, "retained-elf-"+self.architecture)
            actual = {line.split()[0]: elf_machine(bytes.fromhex(line.split()[1])) for line in output.splitlines()}
            if set(actual) != {item["destination"] for item in batch} or any(value != self.profile["machine"] for value in actual.values()):
                raise RuntimeError("retained ELF closure is not independently "+self.architecture)
        loader = manifest["loader"]
        if not re.fullmatch(r"[A-Za-z0-9_.+-]+", loader):
            raise RuntimeError("invalid private loader basename")
        output = self.shell(image, "set -eu; cmp /etc/nsswitch.conf /opt/oneapi-media/nsswitch.conf; "
            f"/opt/oneapi-media/lib/{loader} --inhibit-cache --library-path /opt/oneapi-media/lib "
            "--list /opt/oneapi-media/bin/ffmpeg; "
            f"/opt/oneapi-media/lib/{loader} --inhibit-cache --library-path /opt/oneapi-media/lib "
            "--list /opt/oneapi-media/bin/ffprobe", "private-loader")
        targets = re.findall(r"=>\s+(\S+)", output)
        if not targets or any(not target.startswith("/opt/oneapi-media/lib/") for target in targets):
            raise RuntimeError("actual loader resolves outside private closure")
        donor = root/"donor"
        donor.mkdir()
        self.extract(FFMPEG, {"/usr/local/bin/ffmpeg": "ffmpeg", "/usr/local/bin/ffprobe": "ffprobe"}, donor)
        tools = {}
        for name in ("ffmpeg", "ffprobe"):
            item = next(value for value in records
                        if value["destination"] == "/opt/oneapi-media/bin/"+name)
            with (donor/name).open("rb") as source:
                machine = elf_machine(source.read(20))
            if machine != self.profile["machine"] or sha(donor/name) != item["sha256"]:
                raise RuntimeError("retained tool differs from pinned native donor")
            tools[name] = {"sha256": item["sha256"], "machine": machine}
        save(self.evidence/"retained-manifest.json", manifest)
        save(self.evidence/"retained-verification.json", {"state": "passed", "elf_count": len(elfs),
             "file_count": len(records), "expected_machine": self.profile["machine"], "donor": tools,
             "private_loader": loader, "nss_config_match": True})

    def cleanup(self):
        """cleanup removes only invocation-labelled containers and never global caches or images."""
        if not self.containers_enabled or not self.docker or platform.system() != "Linux":
            return
        try:
            result = subprocess.run([self.docker, "ps", "-aq", "--filter", "label=oneapi.native.owner="+self.token],
                                    timeout=30, capture_output=True, text=True, check=False)
            owned = result.stdout.split()
            if owned and all(re.fullmatch(r"[0-9a-f]{12,64}", item) for item in owned):
                subprocess.run([self.docker, "rm", "-f", *owned], timeout=60,
                               capture_output=True, text=True, check=False)
            self.summary["cleanup"] = {"container_ids": owned, "scope": "invocation-labelled containers only",
                                       "scratch_retained": str(self.work)}
        except (OSError, subprocess.TimeoutExpired) as error:
            self.summary["cleanup"] = {"error": str(error)}


def main():
    """main requires every actual-image qualification before reporting a native pass."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", required=True, type=Path)
    parser.add_argument("--expected-head", required=True)
    parser.add_argument("--timeout", type=int, default=6600)
    parser.add_argument("--architecture", choices=("arm64", "amd64"), default="arm64")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", args.expected_head) or not 300 <= args.timeout <= 10800:
        parser.error("expected-head must be a full SHA; timeout must be 300..10800 seconds")
    qualification = Qualification(args)
    def interrupted(signum, _frame):
        """interrupted persists incomplete qualification on workflow cancellation."""
        raise RuntimeError("qualification interrupted by signal "+str(signum))
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        qualification.setup()
        qualification.source()
        qualification.inputs()
        media = qualification.build("media", qualification.context)
        images = qualification.controls(media)
        images["media"] = media
        images["media-custom"] = qualification.build("media-custom", qualification.context, True)
        cache = qualification.tokenizer()
        for name, image in images.items():
            qualification.runtime(image, cache, name.endswith("-custom"))
        qualification.media(images, cache)
        transport = qualification.run([sys.executable, "scripts/qualify_media_transport.py", images["original"],
            images["ownership"], images["media"]], "transport", 600, cwd=qualification.context)
        save(qualification.evidence/"transport-receipt.json", json.loads(transport))
        qualification.integrity(media)
        qualification.summary["state"] = "passed"
    except BaseException as error:
        qualification.summary.update(state="incomplete", error=str(error), error_type=type(error).__name__)
    finally:
        qualification.cleanup()
        qualification.summary["elapsed_seconds"] = time.monotonic()-qualification.started
        if platform.system() == "Linux":
            qualification.summary["final_resources"] = qualification.resources()
        qualification.flush()
    print(json.dumps({"state": qualification.summary["state"], "error": qualification.summary.get("error"),
                      "evidence": str(qualification.evidence)}, indent=2))
    return 0 if qualification.summary["state"] == "passed" else 1


if __name__ == "__main__":
    sys.exit(main())
