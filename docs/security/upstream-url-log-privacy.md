# Upstream URL privacy in logs

An upstream endpoint is operator-owned configuration. It may contain a provider credential in userinfo, any query parameter, or a fragment. Query-key allowlists are insufficient because providers and custom endpoints can use arbitrary names.

## Boundaries

- Outbound requests retain their original URL, query, headers, and body. Sanitization only changes diagnostic or persistence copies.
- `SanitizeLogUpstreamEndpoint` strips userinfo, all query parameters, and fragments. Malformed, opaque, unsupported-scheme, and invalid hostless absolute URLs fail closed. HTTP(S), WS(S), and valid root-relative paths retain useful route diagnostics.
- Billing sanitizes the final composed metadata regardless of whether the URL came from `BillingDetail.UpstreamEndpoint` or caller-supplied metadata. Token counts and quota values are unchanged.
- `LogMetadata.Value` and direct JSON serialization sanitize persisted/serialized copies without mutating caller-owned maps.
- External log DTOs omit `upstream_endpoint` entirely, including for historical records. This intentionally hides internal provider hostnames as well as credentials across list, cursor, token-scoped, and export paths that use the common DTO conversion.
- Request and response diagnostics use the same sanitizer, including the legacy direct-HTTP audio path and both base-URL and prepared-URL channel-test diagnostics.
- Request-construction and transport `url.Error` values expose sanitized URLs while preserving their underlying cause for `errors.Is`, `errors.As`, and timeout classification. The shared `SanitizeRequestURLError` helper also protects audio failures.

This does not rewrite old database rows or delete old application logs. Existing leaked credentials require separate rotation and retention/remediation decisions. The read boundary prevents continuing to expose an old endpoint through the log DTO, including administrative DTO responses.

## Behavioral red/green evidence

All experiments ran on 2026-10-02 against local HTTP servers or deterministic transports, without live provider calls. A compilation error, race, or fixture panic is not accepted as a red control.

The core red phase reproduced five groups: strict URL parsing, historical log DTO serialization, both billing metadata sources, real HTTP request diagnostics, and captured/buffered response diagnostics. The original PR sanitizer remained installed for that control, demonstrating its opaque-URL gap. A missing tracing fixture was corrected before accepting the request-diagnostic red result. Core green tests passed three race-enabled runs and `go vet ./...` passed. Evidence: [run 37055131341](https://github.com/Laisky/one-api/actions/runs/37055131341), artifact `security-review-442`; code `ac08bbf8d60b7a7056a5c176156dc98046983b7c`.

The supplemental red phase restored only the old audio and channel-test production paths. `TestReviewAudioURLPrivacy` failed its success, transport-error, and malformed-URL cases; `TestReviewChannelProbeURLPrivacy` also failed on actual diagnostic fields. After repair, **28 top-level tests and their subtests** passed three race-enabled runs, with `go vet ./...` passing. Evidence: [run 37056499990](https://github.com/Laisky/one-api/actions/runs/37056499990), artifact `security-review-442-extra`; code `53e4b39d92600b3f254bb9c9bc5b13e16b15a3ce`.

The extended suite verifies the original audio request/response, exact charge of 25 quota units for the fixture, preserved refund behavior on failed dispatch, channel-probe endpoint overrides, original query/header dispatch, and the existing transport-error cause/channel-identity matrix. Its older partially-redacted-query expectation now matches the stricter all-query removal policy; the original dispatch and error-cause assertions remain intact.

```sh
go test -race -count=3 -timeout=15m \
  -run '^Test(Review|UpstreamTransportFailureURLPrivacy|ProtocolAuditAudioHTTP|ProbeUsesDeclaredChatSurface|DoRequestHelperRedactsQueryCredentialsWithoutChangingDispatch|LogMetadata|LogsToResponses|SanitizeLogUpstreamEndpoint|PostConsumeQuotaDetailedSanitizesUpstreamEndpoint)' \
  ./controller ./relay/controller ./relay/adaptor ./relay/billing ./model
go vet ./...
```

Artifacts are subject to retention. Permanent tests remain in the repository's normal suite; all temporary transfer/validation files are removed from the final tree. Tests preserve useful diagnostic events and original request dispatch instead of suppressing logging or modifying authentication to make privacy assertions pass.
