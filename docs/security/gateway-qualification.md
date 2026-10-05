# Gateway guide qualification

The guide's pinned Envoy Gateway v1.9.2 charts were rendered and installed in an
isolated local Kubernetes v1.36.4 cluster. Cilium v1.20.2 enforced the actual
NetworkPolicy. The guide's GatewayClass, Gateway, HTTPRoutes and NetworkPolicy
were applied without substituting permissive test policies. Main's session
provisioning changes from PR #522 are also included.

## Behavioral reproduction and regression

Test-only commit `0a2e9ae8` contains the cluster harness and synthetic backend.
With the unchanged network policy, backend service-name resolution failed with
`socket.gaierror`, while an unselected control pod resolved the same name. TLS,
credential-free redirects, SSE and WebSocket controls passed. This isolates the
failure to the policy rather than a broken cluster resolver or proxy.

The fix permits TCP and UDP port 53 only to the cluster DNS namespace/pod
selector. The unchanged three-test suite then passed. The expanded six-test
suite also passes:

- Verified test-certificate TLS and HTTP-to-HTTPS redirects.
- SSE delivery and WebSocket upgrade through the real Envoy data plane.
- Service DNS resolution from the network-policy-selected backend.
- A control pod cannot directly access the backend, while Envoy still can.
- One synthetic POST is observed once; cancelling an SSE client reaches the backend.
- Cross-namespace route attachment is explicitly rejected as `NotAllowedByListeners`.

The backend is a small retained Python fixture, not One API or a paid provider.
Its image is pinned to the tested platform digest. Certificates are generated
locally and private-key files are restricted and removed by the test harness.

## Reproduction

Use a new, dedicated Kind cluster; never point these tests at a production
cluster. The harness refuses a context other than `kind-oneapi-security-492` and
a Kubernetes API outside loopback. It requires an explicit kubeconfig.

The tested tooling is Kind v0.33.0 with node image
`kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed`.
Create it with `networking.disableDefaultCNI: true`, then install Cilium 1.20.2
using `ipam.mode=kubernetes` and one operator replica. Follow the guide's pinned
CRD and controller installation commands, passing the dedicated kubeconfig to
every Helm/kubectl command. The tests deploy the synthetic app and the guide's
gateway/network resources themselves.

```sh
python3 -m unittest discover -s scripts -p test_k8s_gateway_docs.py -v
ONEAPI_GATEWAY_TEST_KUBECONFIG=/absolute/path/to/isolated-kubeconfig \
  python3 -m unittest discover -s scripts -p test_k8s_gateway_cluster.py -v
```

The twelve offline tests run through the existing workflow-contract CI gate.
Cluster tests are opt-in and skip when the explicit environment setting is
absent. Remove the dedicated cluster after testing with `kind delete cluster
--name oneapi-security-492 --kubeconfig /absolute/path/to/isolated-kubeconfig`.

## Limits and deployment acceptance

This is real chart, API-schema, proxy and CNI evidence. It is not a CVE exploit,
cloud LoadBalancer test, public certificate issuance test, production upgrade,
or proof of a particular deployment's DNS cutover/rollback. The guide retains
staged migration, maintained rollback and ownership checks for those
environment-specific steps. NodeLocal DNSCache and distributions with different
resolver labels require the documented DNS-policy adaptation.

Primary installation references:
[Envoy Gateway](https://gateway.envoyproxy.io/docs/tasks/quickstart/),
[Kind](https://kind.sigs.k8s.io/docs/user/quick-start/), and
[Cilium on Kind](https://docs.cilium.io/en/stable/installation/kind/).
