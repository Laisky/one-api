# Public certificate fixtures for GO-2026-6809

These fixtures contain public certificates only. They were downloaded from the
issuer's [public repository](https://repository.emsign.com/) on 2026-10-21 and
converted from DER to PEM without changing the certificate bytes.

| Fixture | Issuer download | Certificate SHA-256 |
| --- | --- | --- |
| `em_dv_tls_ca_g3b.pem` | [EM DV TLS CA - G3B](https://repository.emsign.com/certs/EMDVTLSCAG3B.crt) | `42c4accd12c4f59597925e1a08a7a90102608543809308d50ec427e118ba1493` |
| `emsign_root_tls_ca_g3.pem` | [emSign Root TLS CA - G3](https://repository.emsign.com/certs/roottlscag3.crt) | `7dd78d5f4f13459a83dff9abbba62edbaf6f2d102bf257fd712f4d9f2746ed8d` |

The intermediate's SHA-256 SPKI fingerprint is
`ebed6475c2c2d80fcb199f3fe8dd6e6b3ce60b77eb15d4e42c3dec0c6f5b5105`,
one of the keys distrusted by Go's
[official fix](https://github.com/golang/go/commit/90617fb7ceb59cf08bee7826bb51b8c8fb8c11de)
for [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809).

`TestToolchainX509CertificateTrust` verifies this genuine signed intermediate
directly against its issuing root in an isolated certificate pool. It pins the
verification date inside both certificates' validity periods and checks the
signatures independently, so an expired fixture or invalid signature cannot
masquerade as the security fix. Go 1.27.2 accepts this chain; Go 1.27.3 rejects it
with `x509.NoValidChains`. The valid-chain control creates its own keys at test
runtime and confirms that unrelated certificate validation still works.

The regression is offline. It does not load system roots, retrieve revocation
data, invoke a live service, or contain or use the compromised private key.
