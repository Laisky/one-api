# Proxy relay billing boundary

The `/v1/oneapi/proxy/:channelid/*target` route is intentionally unmetered. It must only forward through an explicitly configured `Proxy` channel with the `Proxy` adaptor API type. A paid provider channel must never enter this path, even when its URL builder currently produces an upstream 404.

`RelayProxyHelper` rejects any other channel/API-type combination with `403 proxy_channel_required` before reading the body, constructing an upstream request, forwarding credentials, or recording a zero-cost consumption log. Ordinary metered completion routes are unchanged. Operators who intentionally provide an unmetered proxy must configure a dedicated Proxy channel; it is not a substitute for metered model routing.

GitHub Models path normalization is also disabled in Proxy mode. This is defense in depth, not the accounting authorization boundary.

## Behavioral regression coverage

`TestReviewProxyRejectsPaidChannels` exercises the actual controller against a local HTTP upstream and a SQLite accounting fixture. It covers OpenAI-compatible, OpenAI, Azure, GitHub Models, and mismatched channel/API types. Rejections must produce zero upstream requests and leave the user balance unchanged.

`TestReviewProxyPreservesExplicitProxyChannel` is the positive control: an intentionally configured Proxy channel still forwards the path, query, body, and credential header, with the established zero-quota behavior.

`TestReviewGitHubProxyURLDoesNotNormalize` protects proxy path/query construction independently of the controller guard.

## Observed red/green evidence

On 2026-10-02, the new rejection test failed all six negative cases when only the controller guard was restored to the pre-fix implementation. The existing GitHub URL-normalization fix remained present in this control. Restoring the guard made the suite pass three consecutive runs under the race detector. `go vet ./...` also passed.

The experiment used baseline `d7fa5f35ebf3dd6072eace68bd0178196baa0823`; its validated revision is `fbd39278fd7ae5f1897b2552f22c33241f8a2a61`. Logs and summaries are attached to [validation run 37054260780](https://github.com/Laisky/one-api/actions/runs/37054260780), artifact `security-review-443` (subject to artifact retention).

```sh
go test -race -count=3 -timeout=10m -run '^TestReview' ./relay/controller ./relay/adaptor/openai
go vet ./...
```

These are local-upstream behavioral tests, not requests to a live provider. They prove that the gateway's unmetered controller must enforce its own channel boundary; they do not depend on a provider accepting a particular malformed proxy URL.
