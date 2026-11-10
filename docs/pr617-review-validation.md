# PR617 review validation — 2026-11-10

Task evidence ID: 20261110-pr617-review. Base: 9b86a9ea16047e8249f03ddab6baefc6e8beb3bd.
Execution: dev, Go 1.27.3. No merge, production action, live paid inference, credentials, or entitlement changes.

## Behavioral evidence

- Native Claude 5.2: both models accepted fast, unknown, boolean and null speed values before the fix (8 failing boundary cases). Standard/absent speed and opaque replay controls passed. The final shared HTTP preparation boundary now rejects unsupported speed before dispatch and retains valid bodies byte-for-byte.
- Public native and Foundry opaque-deployment dispatch tests verify rejection before HTTP setup. Vertex uses this same boundary.
- AWS: 10 advertised model/effort pairs failed public conversion before the fix; 12 unsupported-region synchronous/stream cases invoked the fixture transport rather than rejecting. An initial streaming fixture was corrected and the RED run repeated before treating these as product failures.
- AWS compatibility controls cover old model validation, old routing, explicit profile ARN overrides, opaque signed thinking, and provider access errors without substituted models or retries.

- Vertex: both new models exposed unverified defaults and reached URL setup before explicit-tariff rejection in RED. The fix reuses the shared pricing validator, requires complete operator tariffs including cache categories, preserves discovery and existing stored configuration, and resolves mapped names/time windows consistently with billing. All four public pricing regressions passed.

## Acceptance scope

Complete native Anthropic, Foundry, AWS and Vertex adaptor subtree suites passed with fresh tests using go test ./relay/adaptor/anthropic ./relay/adaptor/aws/... ./relay/adaptor/vertexai/... ./relay/adaptor/azure -count=1. Targeted go vet passed for the same paths. Final added Vertex mapped-name/window tests also passed separately. The repository quick gate passed all 28 selected essential tests and formatting. The quick gate does not include adaptor suites and is not evidence of their execution; the separately run commands are the adaptor evidence.

The automatic quick-gate budget/selection remains unchanged. These behavior regressions remain part of the retained manual adaptor/full qualification, rather than silently expanding cold adapter compilation beyond its documented budget.

## Remaining operator qualification

Local fixtures do not establish live model access, provider safety/retention consent, region availability or negotiated tariffs. Do not accept or change those settings automatically. Revalidate each provider before release, using current operator-approved pricing and access. Full repository race/database qualification and live inference have not been performed in this task.

## History recording limitation

Native Laisky FileIO tools were unavailable when checked before substantive fixes. No FileIO history search, write, or readback occurred, and no search is claimed retroactively. This checked-in report and PR description preserve available evidence but do not satisfy the requested FileIO closeout requirement. Search terms to use once available: one-api, PR617, Fable 5.2, Mythos 5.2, fast speed billing, Bedrock reasoning effort, GovCloud profile, Vertex pricing.

## Follow-up review B/C/D — 2026-11-10

The native Claude controller's complete request path was tested with finite user
and token balances, a middleware-style request-cost row, and consume logging
enabled. Before the fix, all three rejected requests made zero upstream calls
and eventually restored balances, but provisional log quota remained nonzero:
1312 fixture quota units for native fast, 90 for Vertex missing tariffs, and
1312 for Vertex fast with complete valid tariffs. The valid-tariff speed control
prevents one validation error from masking the other. An initial missing
request-cost fixture row was corrected and RED repeated before diagnosis.

Provider qualification and residency checklist changes are documentation only:
no account access, retention consent, credential, routing configuration,
production traffic or live inference has been changed. The explicit profile
override remains authoritative; automatic commercial-region global routing
does not establish source-region residency.

FileIO discovery was repeated before this follow-up; no native FileIO tools
were exposed. History search/save/readback remains unavailable. This follow-up
is not represented as completed FileIO closeout.

The fix applies the adaptor's existing RequestPricingValidator before native
quota admission and uses one durable, owned refund path for any terminal
unforwarded local failure. It reconciles balances, provisional logs and request
costs before the audit fallback. Already-forwarded/paid work and submitted
response settlement remain excluded. Cancellation still settles; a durable
refund outage retains a pending recovery intent and charged records, blocks
replay, and never fabricates a successful credit or zero cost.

Follow-up acceptance: full go test ./relay/controller -count=1 passed (66.760s);
go vet ./relay/controller passed; make test-quick passed all 28 selected tests
and formatting of 2917 tracked Go files. Dedicated native/controller rejection,
refund-failure, cancellation, duplicate cleanup, forwarded-work and settlement
ownership controls passed. None of these claims imply full repository race or
live provider qualification.

Targeted race qualification also passed (3.409s):
go test -race ./relay/controller -run 'TestClaude52Local|TestClaude431Review|TestBillingAuditSafetyNet_DBAccounting' -count=1.
