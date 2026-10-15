# Channel suspension alert investigation: 20261015-01

Status: the historical incident remains unattributed. The revised caller-controlled
alert transport, explicit application protections and complete suspension
diagnostics passed focused acceptance and final affected-package qualification.
The PR remains Draft for review.

This is the confirmed unique incident ID. Earlier executor labels and evidence
directories containing `20261015-032003Z` refer to this same October 15 event.
Billing `20261012-09` and completed incident `20261009-04` remain separate.

## Historical evidence and limits

The reported webhook event has logger `one-api.gin`, timestamp
`2026-10-15T03:20:03.913330639Z`, message
`ability suspended due to server error (5xx)`, and fields `{}`.

Read-only checks found b1 container `vps_oneapi_1` healthy, with zero restarts,
at source revision `f77867e06e28c8c863f0685e40d198bb41b66db8`. It started at
`2026-10-14T17:20:49.894554782Z`. The final check was at
`2026-10-15T12:33:15Z`; this does not prove any particular ability recovered.

The 03:19:45–03:20:30 UTC Docker query returned no records. Retained coverage
began at `2026-10-15T04:19:35.570182744Z`, after the alert. Channel, model,
underlying cause, caller impact, persistence result and recovery remain unknown.
No link to the October 9 channel/model is established.

At the runtime revision the webhook uses go-utils v6.4.0's default-deny field
hook, explaining `{}` despite populated call-site context. The success message
also precedes the database update. It therefore proves neither an actual
upstream HTTP 5xx response nor a successfully persisted pause.

The branch receives a normalized 500–599, potentially synthesized from a
transport/body-read failure. It targets one `(group, OriginalModel, channel_id)`
ability. Nominal duration defaults to 30 seconds; the named duration setting was
unset at inspection. `SYNC_FREQUENCY` was 30 seconds; effective memory caching
was not established. Actual suspension and recovery duration are not proven.

## Final design: caller controls content

The transport uses standard Zap JSON encoding and the existing `Alert.Send`
queue. There is no field allowlist, secret-key denylist, identifier format/type
filter, hash substitution, namespace-wide omission or custom unsupported-type
drop. Caller fields, ordinary nested namespaces, object/array/reflection/error/
stringer serialization and duplicate-key behavior match ordinary Zap.

`logger.Redact(value)` is explicit opt-in: it returns `[REDACTED]` without
inspecting or serializing the selected value. It affects every sink because the
application chooses the field value before logging. The transport does not
guess which content is private; an unmarked fake `authorization` field survives
its isolated delivery test deliberately.

Retained operational behavior is explicit and tested:

- Existing ERROR floor, queue admission, sender-side rate limiter, alert
  type/token routing, timeout and cancellation lifecycle remain.
- Normal delivery is one standard JSON entry with UTC timestamp, logger,
  severity, message and caller fields.
- A message exceeding 16 KiB becomes valid JSON containing
  `alert_truncated=true`, `original_bytes` and a bounded UTF-8 preview. It never
  silently becomes `{}`. This is an output bound after standard serialization,
  not an allocation sandbox for application-provided serializers.
- The alert core is an independent tee, so local stdout/disk-budget rejection
  no longer silently suppresses alert admission. The existing sender limit
  remains. This admission change is intentional.
- Sending remains asynchronous; `Sync` cannot flush an API that has no flush
  operation. `Close` cancels, so tests wait for delivery before closing/joining.

No shared-library update or access/security configuration change is needed:
the pinned dependency already supports sending completed messages.

## Explicit application policy and audit

The migration audit reviewed 95 controller/middleware/router source files plus
1,015 source files in the other assigned scopes, including 290 direct
ERROR/FATAL/PANIC candidates and nine inherited-field sites. It found these
concrete producers requiring explicit application treatment:

| Producer | Caller-selected treatment | Preserved diagnostics |
| --- | --- | --- |
| Relay request URL | Existing endpoint sanitizer; visible `request_url_redacted=true`; invalid URL gets `[REDACTED]` | Endpoint, model/channel/request context |
| Two complete-body relay-error parser fallbacks | Internal `MessageContainsRawBody` provenance, `json:"-"`; only flagged log message is explicitly redacted | Ordinary structured provider reasons, parser/status error, unchanged client error JSON |
| Proxy configuration / URI-parser failures | Explicitly redact configured URI and credential-bearing parse error | Configuration name, failure category/error type |
| Redis URI parse failures | Explicitly redact URI and URI-bearing parser error | Static parse failure and error type |
| Malformed OpenAI SSE diagnostic | Explicitly redact raw chunk | Parser reason, byte count, unchanged bytes forwarded to API client |
| Value-bearing JSON decoder error in relay/SSE/Resend fallbacks | Explicit opt-in decoder-error helper removes the copied raw numeric literal | Ordinary errors, wrapper context/stack, decoder field/type context and unchanged client behavior |
| Three unparsed Resend response fallbacks | Explicitly redact raw response body in the diagnostic error | Status, length/parser failure; ordinary parsed provider reason unchanged |

Independent review identified a concrete secondary path: Go can copy an
overflowing numeric literal into a JSON decoder error. Redacting only the raw
body or chunk leaves that literal in the error field. The affected application
callers explicitly apply the decoder-error helper to their logging view; the
transport has no implicit error filter.

This is not a universal privacy guarantee. Arbitrary third-party DB/SMTP/SDK
errors and recovered panic values remain caller-authored. Ordinary errors were
not blanket-redacted. Existing DEBUG/WARN payload paths below the ERROR alert
floor were not broadened into this change. The existing middleware dependency's
URL query masking occurs before transport and is not changed by this PR.

## Suspension diagnostics and unchanged policy

The existing caller context now reaches delivery: `trace_id`, `user_id`,
`request_id`, `token_id`, `channel_id`, `channel_name`, `group`,
`origin_model`, `actual_model`, and other fields the application supplies.
Absent fields are not invented. No transport suppression is applied by name.

5xx outcome and generic relay records include:

- `normalized_status_code` and `status_semantics=normalized_relay_status`.
  Existing `status_code` is retained for compatibility; no
  `upstream_http_status` is invented.
- Existing `error_kind` plus `error_kind_basis`, distinguishing an explicit
  classified cause from `normalized_status_fallback`. An inferred `upstream`
  category is not evidence of an observed upstream HTTP status.
- `suspension_outcome=persisted|failed|not_attempted`.
- Actual signed `suspension_duration_ms` when attempted. Zero/negative
  configuration remains visible and unchanged; persisted means the write
  committed, not that a positive active cooldown exists.

Outcome wording now says `ability suspension persisted/failed after relay
failure (normalized 5xx)`. Success follows commit. For normalized 5xx only,
the generic relay log is deferred until return, allowing the complete outcome
to reach the existing one-per-second limiter first. Excluded cases still emit
the generic record with `not_attempted` and no inapplicable duration.

Classification, retry decisions, ability scope, configured duration, cache
behavior, channel status and health metric policy are unchanged.

## Acceptance evidence

All fixtures are synthetic SQLite, in-memory bodies, fake-secret values and
loopback webhook/email HTTP handlers. No paid model request, secret retrieval,
denied telemetry retry, production write, restart or ability re-enable occurred.

Focused RED→GREEN evidence:

- Original ordering controls reproduced rejected UPDATE, missing-row false
  success and precommit logging. Nine exclusions passed unchanged.
- Completeness tests reproduced missing metadata and the generic event winning
  before the outcome. Corrected metadata/order, zero/negative duration and
  not-attempted controls passed with race detection.
- Transparent transport RED reproduced old header-plus-empty-fields delivery;
  GREEN covered seven transport contracts, including real nested serialization,
  caller-selected redaction, ordinary fake-secret fields, duplicate semantics,
  visible truncation and operational lifecycle.
- Explicit relay call-site RED reproduced seven raw-body/URL leaks while three
  ordinary controls passed; corrected call-site and prior policy tests passed.
- Service call-site RED reproduced seven proxy/Redis/SSE cases and three email
  body fallbacks; ordinary email reason control passed. Four corrected packages
  passed their targeted race qualification.
- Numeric JSON RED reproduced copied payloads in relay, normal/oversized SSE
  and Resend errors. The explicit helper/callers passed focused race tests.
  A second RED caught short-literal replacement changing unrelated wrapper
  context; exact typed-diagnostic replacement passed with wrapper/stack and
  original cause preserved.
- The final actual processor → production factory → real one-per-second
  limiter → serialized GraphQL request → loopback delivery acceptance passed
  nine scenarios. These include HTTP 200 followed by synthesized 500, every
  persistence outcome, ordinary reason preservation, explicit raw-body/URL
  redaction and signed durations. Its initial test-only interface compile
  failure is retained separately and is not counted as behavioral RED.

Earlier head `190fb4f` passed its own narrower local and CI qualification; that
head's two-key filter was superseded and is not acceptance for this design.
Its initial broader logger run failed seven file/retention cases because the
harness forced stdout; the same source passed after removing only that override.
Both original failure and corrected results remain retained.

Evidence paths include:
`.test-results/suspension-alert/transparent-20261015-01/`,
`.test-results/suspension-alert-20261015-032003Z/{completeness,callsite-redaction,services-callsite}/`,
and `.test-results/20261015-01/application-delivery/`.

Final local qualification on Go 1.27.2 passed `make test-quick`, fresh race
runs across eight affected packages, `go vet ./...`, entity-response analysis,
formatting and diff checks. All 30 top-level tests in changed test files ran
and passed. Eleven existing cases skipped: one opt-in logger measurement,
two live-provider tests and eight MySQL/PostgreSQL subcases. SQLite/offline
coverage ran; no full-repository race/database qualification is claimed.

The first broad run retained one failing compatibility assertion that expected
implicit field removal. Seven other packages passed. Replacing only that test
with unmarked-field preservation and explicit-redaction cases through both
`SetupEnhancedLogger` sinks made the complete logger package pass. All 2,505
tracked Go source hashes were compared: only that test file changed between
these runs; production source remained identical. The original failure,
corrected logger run, terminal inventory and source comparison are retained in
`.test-results/20261015-01/final-qualification{,-corrected}/`. Numeric followup
evidence is retained in `.test-results/20261015-01/numeric-json-redaction/`.

Exact-head CI results are recorded in the Draft PR description after the feature
branch is pushed. The initial failed evidence is preserved, not relabeled green.

## Remaining incident and release work

The historical RCA needs a retained correlated failure, persistence outcome,
contemporaneous caller/retry result and matched recovery observation. No
available permitted source supplied these; no previously refused path was
retried.

The PR remains Draft. No merge, deployment or channel-health-policy change is
part of this step. Review the application-owned privacy choices and final-head
acceptance evidence before any separately authorized release action.
