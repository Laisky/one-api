"""Opt-in cluster tests for the actual gateway and network-policy guide examples.

Requires the dedicated kind-oneapi-security-492 cluster with Envoy Gateway and
a NetworkPolicy-enforcing CNI installed. The suite never selects a default
kubeconfig, installs a real One API deployment, or calls paid providers.
"""

import base64
import http.client
import json
import os
from pathlib import Path
import socket
import ssl
import subprocess
import tempfile
import time
import unittest

import yaml

from test_k8s_gateway_docs import DOC, blocks

CONFIG = os.environ.get("ONEAPI_GATEWAY_TEST_KUBECONFIG", "")


@unittest.skipUnless(CONFIG, "set ONEAPI_GATEWAY_TEST_KUBECONFIG for isolated cluster tests")
class GatewayClusterTests(unittest.TestCase):
    """GatewayClusterTests verifies rendered resources through an actual proxy and CNI."""

    @classmethod
    def command(cls, *args, input_text=None, timeout=90):
        """command executes bounded kubectl operations only against the explicit cluster."""
        return subprocess.run(["kubectl", "--kubeconfig", CONFIG, *args], input=input_text,
                              text=True, capture_output=True, check=True, timeout=timeout).stdout

    @classmethod
    def apply(cls, document):
        """apply submits a synthetic resource or a guide example to the isolated cluster."""
        return cls.command("apply", "--server-side", "-f", "-", input_text=yaml.safe_dump(document))

    @classmethod
    def setUpClass(cls):
        """setUpClass validates cluster isolation and deploys the actual guide resources."""
        context = cls.command("config", "current-context").strip()
        if context != "kind-oneapi-security-492":
            raise RuntimeError("refusing a cluster other than the dedicated kind fixture")
        config = json.loads(cls.command("config", "view", "--minify", "-o", "json"))
        if not config["clusters"][0]["cluster"]["server"].startswith("https://127.0.0.1:"):
            raise RuntimeError("refusing a non-loopback Kubernetes API")
        cls.temporary = tempfile.TemporaryDirectory(prefix="oneapi-gateway-fixture-")
        cls.addClassCleanup(cls.temporary.cleanup)
        directory = Path(cls.temporary.name)
        cert, key = directory / "cert.pem", directory / "key.pem"
        subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
                        "-keyout", str(key), "-out", str(cert), "-days", "1", "-subj",
                        "/CN=oneapi.yourdomain.com", "-addext", "subjectAltName=DNS:oneapi.yourdomain.com"],
                       check=True, capture_output=True, timeout=30)
        key.chmod(0o600)
        cls.apply({"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": "one-api"}})
        cls.apply({"apiVersion": "v1", "kind": "Secret", "metadata": {"name": "one-api-tls", "namespace": "one-api"},
                   "type": "kubernetes.io/tls", "data": {"tls.crt": base64.b64encode(cert.read_bytes()).decode(),
                                                          "tls.key": base64.b64encode(key.read_bytes()).decode()}})
        cls.apply({"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "gateway-fixture", "namespace": "one-api"},
                   "data": {"server.py": Path(__file__).with_name("k8s_gateway_fixture.py").read_text()}})
        cls.apply({"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "gateway-fixture", "namespace": "one-api"},
                   "spec": {"replicas": 1, "selector": {"matchLabels": {"app": "one-api"}},
                            "template": {"metadata": {"labels": {"app": "one-api"}}, "spec": {
                                "containers": [{"name": "fixture", "image": "python:3.13-alpine@sha256:2d9aefe2fef018a7eb2c13064c89c71929800fd2e5dccdbf52ea5da5bb8d929a", "command": ["python", "/fixture/server.py"],
                                                "volumeMounts": [{"name": "script", "mountPath": "/fixture", "readOnly": True}],
                                                "resources": {"limits": {"cpu": "200m", "memory": "64Mi"}}}],
                                "volumes": [{"name": "script", "configMap": {"name": "gateway-fixture"}}]}}}})
        cls.apply({"apiVersion": "v1", "kind": "Service", "metadata": {"name": "one-api-service", "namespace": "one-api"},
                   "spec": {"selector": {"app": "one-api"}, "ports": [{"port": 80, "targetPort": 3000}]}})
        cls.apply({"apiVersion": "v1", "kind": "Pod", "metadata": {"name": "network-control", "namespace": "one-api"},
                   "spec": {"containers": [{"name": "control", "image": "python:3.13-alpine@sha256:2d9aefe2fef018a7eb2c13064c89c71929800fd2e5dccdbf52ea5da5bb8d929a", "command": ["sleep", "3600"],
                                             "resources": {"limits": {"cpu": "100m", "memory": "32Mi"}}}]}})
        for example in blocks(DOC.read_text(), "yaml"):
            if example.startswith(("# gateway.yaml", "# network-policy.yaml")):
                for resource in yaml.safe_load_all(example):
                    cls.apply(resource)
        cls.command("-n", "one-api", "rollout", "status", "deployment/gateway-fixture", "--timeout=120s", timeout=130)
        cls.command("-n", "one-api", "wait", "pod/network-control", "--for=condition=Ready", "--timeout=60s")
        cls.command("-n", "envoy-gateway-system", "wait", "pod", "-l", "gateway.envoyproxy.io/owning-gateway-name=one-api-gateway",
                    "--for=condition=Ready", "--timeout=120s", timeout=130)
        services = json.loads(cls.command("-n", "envoy-gateway-system", "get", "service", "-l",
                                         "gateway.envoyproxy.io/owning-gateway-name=one-api-gateway", "-o", "json"))
        service = services["items"][0]["metadata"]["name"]
        cls.forward = subprocess.Popen(["kubectl", "--kubeconfig", CONFIG, "-n", "envoy-gateway-system", "port-forward",
                                        "service/" + service, "19443:443", "19080:80"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        cls.addClassCleanup(cls.stop_forward)
        cls.tls = ssl.create_default_context(cafile=str(cert))
        for _ in range(60):
            try:
                with socket.create_connection(("127.0.0.1", 19443), timeout=1):
                    break
            except OSError:
                time.sleep(0.25)
        else:
            raise RuntimeError("local proxy port-forward did not start")

    @classmethod
    def stop_forward(cls):
        """stop_forward terminates and joins the fixture's local port-forward process."""
        cls.forward.terminate()
        cls.forward.wait(timeout=10)

    def connect(self):
        """connect opens a verified TLS socket to the isolated proxy using its test certificate."""
        raw = socket.create_connection(("127.0.0.1", 19443), timeout=5)
        return self.tls.wrap_socket(raw, server_hostname="oneapi.yourdomain.com")

    def test_https_and_redirect(self):
        """test_https_and_redirect verifies TLS routing and a credential-free HTTP redirect."""
        with self.connect() as connection:
            connection.sendall(b"GET / HTTP/1.1\r\nHost: oneapi.yourdomain.com\r\nConnection: close\r\n\r\n")
            self.assertIn(b"200", connection.recv(4096).split(b"\r\n", 1)[0])
        with socket.create_connection(("127.0.0.1", 19080), timeout=5) as connection:
            connection.sendall(b"GET / HTTP/1.1\r\nHost: oneapi.yourdomain.com\r\nConnection: close\r\n\r\n")
            response = connection.recv(4096)
            self.assertIn(b"301", response.split(b"\r\n", 1)[0])
            self.assertIn(b"https://oneapi.yourdomain.com", response)

    def test_stream_and_websocket(self):
        """test_stream_and_websocket verifies unbuffered SSE and WebSocket upgrade through TLS."""
        with self.connect() as connection:
            connection.sendall(b"GET /sse HTTP/1.1\r\nHost: oneapi.yourdomain.com\r\nConnection: close\r\n\r\n")
            response = b""
            while b"data: 1" not in response:
                response += connection.recv(4096)
            self.assertIn(b"text/event-stream", response)
        with self.connect() as connection:
            connection.sendall(b"GET /ws HTTP/1.1\r\nHost: oneapi.yourdomain.com\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
            response = b""
            while b"\x81\x02ok" not in response:
                part = connection.recv(4096)
                self.assertTrue(part, "upgrade closed before its frame")
                response += part
            self.assertIn(b"101", response.split(b"\r\n", 1)[0])

    def test_backend_dns(self):
        """test_backend_dns verifies the guide's egress policy permits service DNS resolution."""
        probe = ["env", "RES_OPTIONS=attempts:1 timeout:1", "python", "-c",
                 "import socket; socket.getaddrinfo('kubernetes.default.svc.cluster.local',443)"]
        self.command("-n", "one-api", "exec", "network-control", "--", *probe, timeout=15)
        result = subprocess.run(["kubectl", "--kubeconfig", CONFIG, "-n", "one-api", "exec",
                                 "deployment/gateway-fixture", "--", *probe], capture_output=True, text=True, timeout=15)
        self.assertEqual(0, result.returncode, result.stderr)

    def request(self, method, endpoint, body=b""):
        """request returns a parsed synthetic HTTP response through verified local TLS."""
        with self.connect() as connection:
            headers = f"{method} {endpoint} HTTP/1.1\r\nHost: oneapi.yourdomain.com\r\nContent-Length: {len(body)}\r\nConnection: close\r\n\r\n"
            connection.sendall(headers.encode() + body)
            with http.client.HTTPResponse(connection) as response:
                response.begin()
                return response.status, response.read()

    def test_untrusted_pod_cannot_reach_backend(self):
        """test_untrusted_pod_cannot_reach_backend proves the CNI enforces the ingress selector."""
        service = json.loads(self.command("-n", "one-api", "get", "service/one-api-service", "-o", "json"))
        address = service["spec"]["clusterIP"]
        probe = f"import urllib.request; urllib.request.urlopen('http://{address}/',timeout=2).read()"
        result = subprocess.run(["kubectl", "--kubeconfig", CONFIG, "-n", "one-api", "exec", "network-control", "--",
                                 "python", "-c", probe], capture_output=True, text=True, timeout=15)
        self.assertNotEqual(0, result.returncode, "untrusted pod reached the protected backend")
        self.assertIn("timed out", result.stderr)
        self.assertEqual(200, self.request("GET", "/")[0], "authorized proxy must still reach backend")

    def test_post_once_and_stream_cancellation(self):
        """test_post_once_and_stream_cancellation checks no duplicate POST and upstream disconnect."""
        _, before = self.request("GET", "/")
        before = json.loads(before)
        self.assertEqual(200, self.request("POST", "/", b"{}")[0])
        _, after = self.request("GET", "/")
        self.assertEqual(before["posts"] + 1, json.loads(after)["posts"])
        with self.connect() as connection:
            connection.sendall(b"GET /sse HTTP/1.1\r\nHost: oneapi.yourdomain.com\r\nConnection: close\r\n\r\n")
            with http.client.HTTPResponse(connection) as response:
                response.begin()
                self.assertEqual(200, response.status)
                self.assertIn(b"data:", response.read(16))
        for _ in range(50):
            _, after = self.request("GET", "/")
            if json.loads(after)["cancelled"] > before["cancelled"]:
                break
            time.sleep(0.1)
        else:
            self.fail("client cancellation did not reach the synthetic backend")

    def test_cross_namespace_attachment_rejected(self):
        """test_cross_namespace_attachment_rejected verifies the guide's Same namespace contract."""
        self.apply({"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": "one-api-outsider"}})
        self.apply({"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
                    "metadata": {"name": "unauthorized", "namespace": "one-api-outsider"},
                    "spec": {"parentRefs": [{"name": "one-api-gateway", "namespace": "one-api", "sectionName": "https"}],
                             "hostnames": ["oneapi.yourdomain.com"], "rules": [{"filters": [{"type": "RequestRedirect",
                                 "requestRedirect": {"hostname": "attacker.invalid"}}]}]}})
        for _ in range(50):
            route = json.loads(self.command("-n", "one-api-outsider", "get", "httproute/unauthorized", "-o", "json"))
            conditions = [condition for parent in route.get("status", {}).get("parents", []) for condition in parent.get("conditions", [])]
            rejected = [condition for condition in conditions if condition["type"] == "Accepted" and condition["status"] == "False"]
            if rejected:
                self.assertEqual("NotAllowedByListeners", rejected[0]["reason"])
                break
            time.sleep(0.2)
        else:
            self.fail("cross-namespace route was not explicitly rejected")
        self.assertEqual(200, self.request("GET", "/")[0])


if __name__ == "__main__":
    unittest.main()
