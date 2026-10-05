"""Offline regression contracts for the copyable Kubernetes gateway guide.

These tests capture install-command arguments with fake executables and inspect
parsed YAML. They do not render a Helm chart, probe a CVE, or qualify a cluster.
Run: python -m unittest discover -s scripts -p test_k8s_gateway_docs.py -v
Dependency: PyYAML, pinned in requirements-k8s-docs.txt.
"""
from __future__ import annotations

import copy
import datetime as dt
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[1]
DOC = ROOT / "docs/manuals/k8s.md"
CHART = "oci://docker.io/envoyproxy/gateway-helm"
CRDS_CHART = "oci://docker.io/envoyproxy/gateway-crds-helm"


def blocks(markdown: str, language: str) -> list[str]:
    """blocks returns fenced examples in markdown for the requested language."""
    pattern = rf"^ {{0,3}}```{re.escape(language)}\s*\n(.*?)^ {{0,3}}```\s*$"
    return re.findall(pattern, markdown, re.MULTILINE | re.DOTALL)


def commands(example: str) -> list[list[str]]:
    """commands returns shell argument vectors without evaluating substitutions."""
    result: list[list[str]] = []
    for line in example.replace("\\\n", " ").splitlines():
        lexer = shlex.shlex(line, posix=True, punctuation_chars="|;&<>")
        lexer.whitespace_split = True
        parts = list(lexer)
        if parts:
            result.append(parts)
    return result


def installer_examples(markdown: str) -> list[str]:
    """installer_examples selects controller installer examples, not prose."""
    return [body for body in blocks(markdown, "bash")
            if any(word in body for word in ("ingress-nginx", "envoyproxy/gateway"))
            and any(re.search(r"\b(install|template|apply|repo)\b", " ".join(cmd))
                    for cmd in commands(body))]


def pinned_version(markdown: str) -> str:
    """pinned_version validates the single reviewed version assignment."""
    assignments = [cmd[1] for body in blocks(markdown, "bash")
                   for cmd in commands(body)
                   if len(cmd) == 2 and cmd[0] == "export"
                   and cmd[1].startswith("ENVOY_GATEWAY_VERSION=")]
    if len(assignments) != 1:
        raise ValueError("exactly one central ENVOY_GATEWAY_VERSION assignment is required")
    version = assignments[0].partition("=")[2]
    if not re.fullmatch(r"v\d+\.\d+\.\d+", version):
        raise ValueError("the gateway must use a stable, explicit patch version")
    return version


def capture(example: str, version: str = "", fail_tool: str = "") -> tuple[int, list[list[str]]]:
    """capture runs audited install commands against isolated recording stubs."""
    audited: list[str] = []
    for cmd in commands(example):
        if cmd == ["set", "-euo", "pipefail"]:
            continue
        stages: list[list[str]] = [[]]
        for part in cmd:
            if part == "|":
                stages.append([])
            else:
                if any(char in part for char in (";", "&", "<", ">", "`")):
                    raise ValueError("unsupported shell metacharacter in installer")
                if "$" in part and part not in ("$ENVOY_GATEWAY_VERSION", "${ENVOY_GATEWAY_VERSION}"):
                    raise ValueError("only the reviewed version variable may be expanded")
                stages[-1].append(version if part.startswith("$") else part)
        for stage in stages:
            if not stage or stage[0] not in ("helm", "kubectl"):
                raise ValueError("installer commands must use only helm or kubectl")
        audited.append(" | ".join(shlex.join(stage) for stage in stages))
    bash = shutil.which("bash")
    if not bash:
        raise RuntimeError("bash is required for installer contract tests")
    with tempfile.TemporaryDirectory(prefix="one-api-gateway-docs-") as tmp:
        directory = Path(tmp)
        log = directory / "calls.jsonl"
        stub = f"#!{sys.executable}\n" + textwrap.dedent('''\
            import json, os, pathlib, sys
            name = pathlib.Path(sys.argv[0]).name
            with open(os.environ["CALL_LOG"], "a", encoding="utf-8") as output:
                output.write(json.dumps([name, *sys.argv[1:]]) + "\\n")
            if os.environ.get("FAIL_TOOL") == name:
                sys.exit(41)
            if name == "helm" and sys.argv[1:2] == ["template"]:
                print("apiVersion: v1\\nkind: List\\nitems: []")
            if name == "kubectl" and "-" in sys.argv[1:]:
                sys.stdin.read()
            ''')
        for name in ("helm", "kubectl"):
            executable = directory / name
            executable.write_text(stub, encoding="utf-8")
            executable.chmod(0o700)
        env = {"PATH": tmp, "HOME": tmp, "KUBECONFIG": str(directory / "absent"),
               "CALL_LOG": str(log), "FAIL_TOOL": fail_tool, "LC_ALL": "C"}
        process = subprocess.run([bash, "--noprofile", "--norc", "-euo", "pipefail", "-c",
                                  "\n".join(audited)], env=env, cwd=tmp,
                                 capture_output=True, text=True, timeout=10)
        calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
        return process.returncode, calls


def gateway_objects(markdown: str) -> list[dict]:
    """gateway_objects returns mapping-shaped resources from fenced YAML."""
    result = []
    for body in blocks(markdown, "yaml"):
        for resource in yaml.safe_load_all(body):
            if isinstance(resource, dict):
                result.append(resource)
    return result


def validate_gateway(objects: list[dict]) -> None:
    """validate_gateway checks TLS, routing, tenancy, and paid-request contracts."""
    def one(kind: str, name: str) -> dict:
        """one returns one uniquely named resource or raises a contract failure."""
        matches = [obj for obj in objects if obj.get("kind") == kind
                   and obj.get("metadata", {}).get("name") == name]
        if len(matches) != 1:
            raise ValueError(f"expected one {kind}/{name}")
        return matches[0]

    gc = one("GatewayClass", "one-api-envoy")
    if gc["spec"]["controllerName"] != "gateway.envoyproxy.io/gatewayclass-controller":
        raise ValueError("unexpected gateway controller")
    gateway = one("Gateway", "one-api-gateway")
    if gateway["metadata"].get("namespace") != "one-api":
        raise ValueError("gateway must use the application namespace")
    if gateway["spec"]["gatewayClassName"] != "one-api-envoy":
        raise ValueError("gateway class reference mismatch")
    listeners = {item["name"]: item for item in gateway["spec"]["listeners"]}
    if set(listeners) != {"http", "https"}:
        raise ValueError("exactly http and https listeners are required")
    for name, protocol, port in (("http", "HTTP", 80), ("https", "HTTPS", 443)):
        listener = listeners[name]
        if (listener["protocol"], listener["port"]) != (protocol, port):
            raise ValueError("invalid listener protocol or port")
        if listener.get("hostname") != "oneapi.yourdomain.com":
            raise ValueError("listeners must have the explicit example hostname")
        if listener["allowedRoutes"]["namespaces"]["from"] != "Same":
            raise ValueError("cross-namespace route attachment must not be enabled")
    tls = listeners["https"]["tls"]
    if tls.get("mode") != "Terminate" or tls.get("certificateRefs") != [
            {"group": "", "kind": "Secret", "name": "one-api-tls"}]:
        raise ValueError("TLS must reference the application certificate Secret")
    for name, section in (("one-api-https", "https"), ("one-api-http-redirect", "http")):
        route = one("HTTPRoute", name)
        if route["metadata"].get("namespace") != "one-api":
            raise ValueError("route namespace mismatch")
        if route["spec"]["parentRefs"] != [{"name": "one-api-gateway", "sectionName": section}]:
            raise ValueError("route must attach to exactly its intended listener")
        if route["spec"]["hostnames"] != ["oneapi.yourdomain.com"]:
            raise ValueError("route hostname mismatch")
        if len(route["spec"]["rules"]) != 1:
            raise ValueError("one explicit route rule is required")
        rule = route["spec"]["rules"][0]
        if section == "http":
            if "backendRefs" in rule or rule.get("filters") != [{"type": "RequestRedirect",
                    "requestRedirect": {"scheme": "https", "port": 443, "statusCode": 301}}]:
                raise ValueError("HTTP must redirect without forwarding credentials")
        else:
            if rule.get("backendRefs") != [{"group": "", "kind": "Service",
                    "name": "one-api-service", "port": 80}]:
                raise ValueError("HTTPS must preserve the One API service destination")
            if rule.get("timeouts") != {"request": "300s", "backendRequest": "300s"}:
                raise ValueError("explicit bounded request timeouts are required")
            if "retry" in rule or "filters" in rule:
                raise ValueError("do not add retry/mirroring or header rewriting to paid requests")
            if rule.get("matches") != [{"path": {"type": "PathPrefix", "value": "/"}}]:
                raise ValueError("the backend path must not be rewritten")
    for obj in objects:
        if obj.get("kind") == "Secret" and obj.get("metadata", {}).get("name") == "one-api-tls":
            raise ValueError("the guide must not publish a reusable TLS private key")


class InstallerHarnessControls(unittest.TestCase):
    """InstallerHarnessControls excludes false positives and unsafe test execution."""

    def test_historical_prose_and_nginx_application_are_not_installs(self) -> None:
        """test_historical_prose_and_nginx_application_are_not_installs preserves benign mentions."""
        sample = "Retired ingress-nginx is not supported.\n```yaml\nimage: nginx:alpine\n```\n"
        self.assertEqual(installer_examples(sample), [])

    def test_multiline_pin_reaches_stub(self) -> None:
        """test_multiline_pin_reaches_stub verifies argument expansion and the recording process."""
        code, calls = capture('helm install eg ' + CHART + ' \\\n --version "$ENVOY_GATEWAY_VERSION"', "v1.9.2")
        self.assertEqual(code, 0)
        self.assertEqual(calls, [["helm", "install", "eg", CHART, "--version", "v1.9.2"]])

    def test_pipeline_failure_is_not_hidden(self) -> None:
        """test_pipeline_failure_is_not_hidden preserves the shell pipefail contract."""
        code, calls = capture('helm template crds ' + CRDS_CHART +
                              ' | kubectl apply --server-side -f -', fail_tool="helm")
        self.assertNotEqual(code, 0)
        self.assertEqual(len(calls), 2)

    def test_unexpected_commands_are_rejected(self) -> None:
        """test_unexpected_commands_are_rejected prevents an actual cluster or shell escape."""
        for example in ('curl https://example.com', 'helm install eg $(curl bad)',
                        'helm install eg `id`', 'helm install eg x; id', '/usr/bin/kubectl get secrets'):
            with self.subTest(example=example), self.assertRaises(ValueError):
                capture(example)

    def test_floating_version_is_rejected(self) -> None:
        """test_floating_version_is_rejected prevents implicit latest and development tags."""
        for version in ("latest", "v1.9", "v0.0.0-latest", "v1.9.2-rc.1"):
            with self.subTest(version=version), self.assertRaises(ValueError):
                pinned_version(f"```bash\nexport ENVOY_GATEWAY_VERSION={version}\n```\n")


class KubernetesGatewayGuide(unittest.TestCase):
    """KubernetesGatewayGuide checks the actual shipped guide rather than a copied fixture."""

    @classmethod
    def setUpClass(cls) -> None:
        """setUpClass reads the repository guide once for each test process."""
        cls.markdown = DOC.read_text(encoding="utf-8")

    def test_no_retired_install_target_reaches_tools(self) -> None:
        """test_no_retired_install_target_reaches_tools reproduces copyable unsafe installation."""
        examples = installer_examples(self.markdown)
        self.assertTrue(examples, "removing all installation examples is not an acceptable fix")
        version = "v1.9.2"
        for index, body in enumerate(examples):
            with self.subTest(example=index):
                code, calls = capture(body, version)
                self.assertEqual(code, 0)
                retired = [call for call in calls if any("ingress-nginx" in arg for arg in call)]
                self.assertEqual(retired, [], f"retired installation reached fake tools: {retired}")

    def test_controller_and_crd_pins_are_centralized(self) -> None:
        """test_controller_and_crd_pins_are_centralized checks both supported ownership paths."""
        version = pinned_version(self.markdown)
        minor = ".".join(version.split(".")[:2])
        self.assertEqual(minor, "v1.9", "a new release line requires an explicit compatibility review")
        self.assertGreaterEqual(int(version.split(".")[2]), 2)
        calls = []
        for body in installer_examples(self.markdown):
            code, observed = capture(body, version)
            self.assertEqual(code, 0)
            calls.extend(observed)
        chart_calls = [call for call in calls if CHART in call or CRDS_CHART in call]
        self.assertEqual(len(chart_calls), 3, "expect two alternative CRD paths and one controller install")
        for call in chart_calls:
            self.assertIn("--version", call)
            self.assertEqual(call[call.index("--version") + 1], version)
        controller = next(call for call in chart_calls if CHART in call)
        self.assertIn("crds.enabled=false", controller)
        crds = [call for call in chart_calls if CRDS_CHART in call]
        self.assertTrue(any("crds.gatewayAPI.enabled=false" in call for call in crds))
        self.assertTrue(any("crds.gatewayAPI.enabled=true" in call
                            and "crds.gatewayAPI.channel=standard" in call for call in crds))

    def test_review_is_dated_and_support_window_is_not_expired(self) -> None:
        """test_review_is_dated_and_support_window_is_not_expired forces lifecycle reevaluation."""
        match = re.search(r"gateway-reviewed: (\d{4}-\d{2}-\d{2}); review-before: (\d{4}-\d{2}-\d{2})", self.markdown)
        self.assertIsNotNone(match, "the controller choice needs a dated review window")
        reviewed, deadline = (dt.date.fromisoformat(value) for value in match.groups())
        today = dt.datetime.now(dt.timezone.utc).date()
        self.assertLessEqual(reviewed, today)
        self.assertLess(reviewed, deadline)
        self.assertLessEqual(deadline, dt.date(2027, 2, 14))
        self.assertLess(today, deadline, "review the supported release before using this example")

    def test_gateway_resource_relationships(self) -> None:
        """test_gateway_resource_relationships checks parsed TLS and route semantics."""
        validate_gateway(gateway_objects(self.markdown))

    def test_policy_allows_only_this_gateway_to_backend(self) -> None:
        """test_policy_allows_only_this_gateway_to_backend checks namespace and pod selectors together."""
        policies = [obj for obj in gateway_objects(self.markdown) if obj.get("kind") == "NetworkPolicy"]
        self.assertEqual(len(policies), 1)
        peer = policies[0]["spec"]["ingress"][0]["from"][0]
        self.assertEqual(peer["namespaceSelector"]["matchLabels"],
                         {"kubernetes.io/metadata.name": "envoy-gateway-system"})
        self.assertEqual(peer["podSelector"]["matchLabels"],
                         {"gateway.envoyproxy.io/owning-gateway-namespace": "one-api",
                          "gateway.envoyproxy.io/owning-gateway-name": "one-api-gateway"})

    def test_policy_preserves_cluster_dns(self) -> None:
        """test_policy_preserves_cluster_dns retains the cluster-confirmed resolver fix."""
        policy = next(obj for obj in gateway_objects(self.markdown) if obj.get("kind") == "NetworkPolicy")
        dns = [rule for rule in policy["spec"]["egress"]
               if {("UDP", 53), ("TCP", 53)} <= {(port.get("protocol"), port["port"]) for port in rule.get("ports", [])}]
        self.assertEqual(len(dns), 1)
        peer = dns[0]["to"][0]
        self.assertEqual(peer["namespaceSelector"]["matchLabels"], {"kubernetes.io/metadata.name": "kube-system"})
        self.assertEqual(peer["podSelector"]["matchLabels"], {"k8s-app": "kube-dns"})

    def test_resource_mutations_are_rejected(self) -> None:
        """test_resource_mutations_are_rejected keeps the YAML validator from passing vacuously."""
        objects = gateway_objects(self.markdown)
        validate_gateway(objects)
        mutations = (
            ("Gateway", "one-api-gateway", "namespace"),
            ("Gateway", "one-api-gateway", "routes"),
            ("HTTPRoute", "one-api-https", "retry"),
            ("HTTPRoute", "one-api-http-redirect", "backend"),
            ("HTTPRoute", "one-api-https", "timeout"),
        )
        for kind, name, mutation in mutations:
            candidate = copy.deepcopy(objects)
            obj = next(item for item in candidate if item.get("kind") == kind
                       and item.get("metadata", {}).get("name") == name)
            if mutation == "namespace":
                obj["metadata"]["namespace"] = "other"
            elif mutation == "routes":
                obj["spec"]["listeners"][0]["allowedRoutes"]["namespaces"]["from"] = "All"
            elif mutation == "retry":
                obj["spec"]["rules"][0]["retry"] = {"attempts": 3}
            elif mutation == "backend":
                obj["spec"]["rules"][0]["backendRefs"] = [{"name": "one-api-service", "port": 80}]
            else:
                obj["spec"]["rules"][0]["timeouts"]["request"] = "0s"
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                validate_gateway(candidate)


if __name__ == "__main__":
    unittest.main(verbosity=2)
