# Upstream URL privacy in logs

An upstream endpoint is operator-owned configuration. It may contain a provider credential in userinfo, any query parameter, or a fragment. Query-key allowlists are insufficient because providers and custom endpoints can use arbitrary names.

## Boundaries

- Outbound requests retain their original URL, query, headers, and body. Sanitization only changes diagnostic or persistence copies.
- `SanitizeLogUpstreamEndpoint` strips userinfo, all query parameters, and fragments. Malformed, opaque, unsupported-scheme, and invalid hostless absolute URLs fail closed. HTTP(S), WS(S), and valid root-relative paths retain useful route diagnostics.
- Billing sanitizes the final composed metadata regardless of whether the URL came from `BillingDetail.UpstreamEndpoint` or caller-supplied metadata. Token counts and quota values are unchanged.
- `LogMetadata.Value` and direct JSON serialization sanitize persisted/serialized copies without mutating caller-owned maps.
- External log DTOs omit `upstream_endpoint` entirely, including for historical records. This intentionally hides internal provider hostnames as well as credentials across list, cursor, token-scoped, and export paths that use the common DTO conversion.
- Request and response diagnostics use the same sanitizer. Request-construction and transport `url.Error` values expose sanitized URLs while preserving their underlying cause for `errors.Is` and `errors.As`.

This does not rewrite old database rows or delete old application logs. Existing leaked credentials require separate rotation and retention/remediation decisions. The read boundary prevents continuing to expose an old endpoint through the log DTO.

## Behavioral evidence

On 2026-10-02, five regression groups failed against restored vulnerable production paths with assertion failures: strict URL parsing, historical log DTO serialization, both billing metadata sources, real HTTP request diagnostics, and captured/buffered response diagnostics. The original PR sanitizer remained installed for the red control, demonstrating its opaque-URL gap. A missing tracing fixture was corrected before accepting the request-diagnostic red result; a panic was not treated as proof of a vulnerability.

The same tests, related existing metadata/HTTP compatibility tests, and transport-error regressions passed three consecutive runs with `-race`. `go vet ./...` passed. Evidence is in [validation run 37055131341](https://github.com/Laisky/one-api/actions/runs/37055131341), artifact `security-review-442` (subject to retention). Baseline: `d7fa5f35ebf3dd6072eace68bd0178196baa0823`; validated code: `ac08bbf8d60b7a7056a5c176156dc98046983b7c`.

```sh
go test -race -count=3 -timeout=10m \
  -run '^Test(Review|DoRequestHelperRedactsQueryCredentialsWithoutChangingDispatch|LogMetadata|LogsToResponses|SanitizeLogUpstreamEndpoint|PostConsumeQuotaDetailedSanitizesUpstreamEndpoint)' \
  ./model ./relay/billing ./relay/adaptor ./relay/controller
go vet ./...
```

Tests preserve useful diagnostic events and assert original request dispatch, rather than suppressing logs or sanitizing the actual authenticated request to make the tests pass.
