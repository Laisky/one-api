# Proxy relay billing boundary

The `/v1/oneapi/proxy/:channelid/*target` route is intentionally unmetered. It must only forward through an explicitly configured `Proxy` channel with the `Proxy` adaptor API type. A paid provider channel must never enter this public arbitrary-target path, even when its URL builder currently produces an upstream 404.

`RelayProxyHelper` rejects any other channel/API-type combination with `403 proxy_channel_required` before reading the body, constructing an upstream request, forwarding credentials, or recording zero-cost consumption. Neither its mode argument nor the metadata mode can waive this boundary. GitHub Models path normalization is separately disabled in Proxy mode as defense in depth.

## Settled video operations are a different entrypoint

Video creation remains in `RelayVideoHelper` and its existing quota admission/settlement logic. Subsequent retrieval/deletion does not create another paid task. These video routes retain the existing `BindAsyncTaskChannel` ownership/model/channel checks and call the private forwarding implementation directly, rather than entering the public arbitrary-target proxy controller.

The first guard placement also blocked these legitimate video reads because they reused `RelayProxyHelper`. The full repository CI exposed this compatibility regression. The fix separates the controller entrypoints; it does **not** add a mode-based exception to the public proxy guard. Existing prices, creation accounting, authenticated task ownership, and no-double-charge polling behavior remain unchanged.

## Behavioral regression evidence

All controls use local HTTP upstreams and SQLite accounting, not live providers.

`TestReviewProxyRejectsPaidChannels` covers OpenAI-compatible, OpenAI, Azure, GitHub Models, and mismatched channel/API types. Each combination must produce zero upstream requests and leave the user balance unchanged. It now repeats the check with Proxy, Videos, and ChatCompletions mode arguments and metadata, so a forged mode cannot bypass the public boundary.

`TestReviewProxyPreservesExplicitProxyChannel` confirms that a deliberately configured Proxy channel still forwards path, query, body, and credentials with established zero-quota behavior. `TestReviewGitHubProxyURLDoesNotNormalize` protects URL construction independently.

On 2026-10-02, restoring only the old public proxy controller made all six original rejection cases fail while retaining the original GitHub URL fix. Restoring the guard made three race-enabled runs pass; `go vet ./...` passed. Evidence: [run 37054260780](https://github.com/Laisky/one-api/actions/runs/37054260780), artifact `security-review-443`; code `fbd39278fd7ae5f1897b2552f22c33241f8a2a61`.

The subsequent compatibility control reproduced 29 failing Zhipu/XAI video subcases against the overly broad initial guard. After entrypoint separation, the extended suite passed **12 top-level tests and their subtests, three consecutive race-enabled runs**, including creation/polling ledgers, accepted disconnects, admission rejection, owner/model authorization, and all proxy controls. `go vet ./...` passed. Evidence: [run 37056990511](https://github.com/Laisky/one-api/actions/runs/37056990511), artifact `security-review-443-video-compat`; code `6b1ffd7bc4cbf21ef3601c29b545464c88a7f67c`.

```sh
go test -race -count=3 -timeout=15m \
  -run '^Test(Review|ProtocolAuditZhipuVideoLedger|XAIVideo|BindAsyncTaskChannel|GetRequestURLForOpenAICompatible)' \
  ./relay/controller ./relay/adaptor/openai ./middleware
go vet ./...
```

Artifacts are subject to retention. Temporary validation workflows are removed from the final tree; permanent tests run under the existing repository CI. Intentionally unmetered services must use a dedicated Proxy channel, not a paid model channel accessed through the arbitrary-target proxy URL.
