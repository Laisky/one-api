# Channel suspension alert investigation: 20261015-032003Z

Status: incident cause and affected ability remain unverified. A separate,
reproducible success-log ordering defect is being qualified. This record does
not reopen the completed October 9 investigation.

## Report and runtime evidence

The reported webhook event has logger `one-api.gin`, timestamp
`2026-10-15T03:20:03.913330639Z`, message
`ability suspended due to server error (5xx)`, and structured fields `{}`.

Read-only checks on b1 at `2026-10-15T12:04:31Z` established:

- Container `vps_oneapi_1` was running and healthy, with zero restarts.
- Its source revision was `f77867e06e28c8c863f0685e40d198bb41b66db8`.
- It started at `2026-10-14T17:20:49.894554782Z`, before the reported event.
- A Docker log query for 03:19:45 through 03:20:30 UTC returned no records.
- The retained Docker log boundaries were
  `2026-10-15T04:19:35.570182744Z` and
  `2026-10-15T12:01:35.571933754Z`. The event predates retained coverage.
- The named nonsecret setting `CHANNEL_SUSPEND_SECONDS_FOR_5XX` was unset;
  the source default is 30 seconds. `SYNC_FREQUENCY` was 30 seconds.
  `MEMORY_CACHE_ENABLED` and `ENABLE_METRIC` were unset.

Current container health is not evidence that this particular ability recovered,
that retries succeeded, or that no caller experienced an error. No provider
request, replay, ability re-enable, service restart or production write was made.

## What the event establishes

At the runtime revision, `controller/relay_error.go` emits the message after a
normalized 500–599 error reaches the suspension branch and survives its
internal-infrastructure, internal-adaptor, user-error, cancellation and
retry-suggestion exclusions. A transport failure can become a normalized gateway
5xx; this message does not prove an upstream HTTP 5xx response.

The branch targets exactly one `(group, OriginalModel, channel_id)` ability.
`model.SuspendAbility` sets its `suspend_until` and requires one affected row.
It does not itself disable the whole channel or change the ability's enabled
flag. SQL selection admits an enabled ability after its suspension expires.
Memory routing refreshes periodically, so the configured sync interval can delay
both selection changes and recovery visibility if memory caching is enabled.
An unset memory-cache environment flag does not establish effective cache state:
Redis-enabled startup also enables memory caching.

The success-worded log currently precedes the database update. A missing ability
or failed update can therefore produce both the success claim and
`failed to suspend ability for 5xx`. The supplied event alone proves neither a
successful pause nor its duration. This source defect is not established as the
cause of this specific incident because its matching persistence outcome is
unavailable.

## Why the webhook fields are empty

Logger setup installs the pinned go-utils v6.4.0 alert hook without a remote
field allowlist. Its default-deny field policy encodes an empty map while
preserving logger, timestamp, level and message. The suspension call already
provides structured status, rationale, duration and available request context.

Stdout retains console-encoded context. The separate OTLP path exports its
approved fields and uses the body `application log`. The observed original
message plus empty map is consistent with the webhook policy; it does not
demonstrate lost inherited fields. The user subsequently requested trace ID and
user ID in alerts. A separate, narrowly scoped candidate will export only
validated correlation identifiers; it will not export arbitrary structured
fields, raw errors, payloads or credentials.

## Scoped correction and acceptance

Emit the existing 5xx success event only after `SuspendAbility` succeeds. Keep
the failure event, original success metadata, health-event emission, routing
classification, suspension duration and retry behavior intact. Other suspension
branches and health-policy proposals are outside this change.

Acceptance requires isolated SQLite fixtures to prove:

1. Failed or missing-row updates produce no suspension-success event and leave
   suspension state unchanged.
2. A successful event observes an already persisted timestamp and changes only
   the selected ability.
3. Non-5xx and retry-suggestion exclusions produce no false suspension event.

The correlation candidate must validate trace identifiers before allowing them
through: the current Gin trace field can originate in an unvalidated incoming
header. It must accept only the supported bounded identifier syntax and positive
native integer user IDs, preserve inherited request logger fields, reject
incompatible types without invoking serializers, and keep unrelated fields
excluded. Tests use only a loopback webhook and synthetic identifiers.

This concerns structured fields. Existing alert message text is a separate
privacy boundary and is not globally sanitized by this change. No claim of
universal message redaction is made.

Controller regression qualification used Go 1.27.2 and
`go test -race -count=1 -timeout=5m -run
^TestProcessChannelRelayError5XX -json ./controller` on isolated SQLite fixtures.

- Baseline: exit 1; rejected UPDATE, missing row and precommit success logging
  failed as expected. All nine exclusion controls passed.
- Corrected code: exit 0; all three top-level tests / 12 cases passed with race
  detection enabled.
- Ignored local evidence is retained in
  `.test-results/suspension-alert-20261015-032003Z/{red,green}.*`.

Broader and correlation validation results will be recorded after execution.
No full-repository race/database qualification is claimed by this investigation.

## Remaining incident evidence

The exact channel, group/model, normalized error class, persistence outcome,
retry result and caller impact remain unknown. Establishing these requires an
already authorized retained record covering the original timestamp, followed by
a correlated ability-state or ordinary-traffic result. No previously refused
telemetry/ledger path, new credential, paid request or unsupported browser
operation was retried. The historical October 9 channel/model is not evidence
for this event.

Keep the implementation PR Draft pending review of this acceptance evidence.
Any future production action requires a separately reviewed plan; no deployment
or policy change is part of this investigation.
