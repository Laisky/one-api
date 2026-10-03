# Realtime credential minting is disabled

The gateway must observe and settle billable Realtime traffic. An upstream ephemeral credential lets a client communicate directly with a provider, outside that accounting boundary. The public `POST /v1/realtime/sessions` route is therefore not registered. The supported path remains the metered `GET /v1/realtime` WebSocket endpoint authenticated with a one-api bearer token.

The retired `RelayRealtimeSessions` handler also fails closed with `403 realtime_sessions_disabled`, without metadata/database access or an upstream request. This is defense in depth against a future route alias or direct handler reuse; the original route removal already closed that specific public route.

## Regression controls

- The actual Gin relay router and production not-found handler reject session minting, trailing-slash and encoded variants, transcription sessions, client secrets, and calls with JSON 404 responses, both without authorization and with invalid authorization. The metered WebSocket route remains registered.
- A local HTTP upstream counts calls made by a direct handler invocation. Restoring only the former handler causes the zero-upstream-call assertion to fail; restoring the disabled handler makes it pass. The disabled path is also tested without channel metadata.
- The Gemini compatibility test verifies the uniform disabled policy and the supported WebSocket/one-api-token remediation text.

The route assertion parses JSON keys rather than looking for a `client_secret` substring: a normal not-found error may quote `/v1/realtime/client_secrets` without returning a credential. This false-positive test assertion was corrected without changing production behavior.

## Observed validation

On 2026-10-02, the handler red control failed by observing a real local-upstream call. The revised behavior, related Realtime accounting regressions, router tests, and Gemini compatibility checks passed three consecutive race-enabled runs. `go vet ./...` passed. Evidence is in [run 37055429739](https://github.com/Laisky/one-api/actions/runs/37055429739), artifact `security-review-441` (subject to retention). Validated code: `ff0313d533ebccf7f5f35f4b1191aadbc66dc2c7`; baseline: `d7fa5f35ebf3dd6072eace68bd0178196baa0823`.

```sh
go test -race -count=3 -timeout=10m \
  -run '^Test(Review|GeminiLiveEphemeralRejectionExplainsSupportedTransport|RelayRealtimeSessionsHandlerSignature)' \
  ./controller ./router
go vet ./...
```

Temporary source-transfer and validation tooling is not part of the final tree. The permanent regression tests run under the existing repository workflows.
