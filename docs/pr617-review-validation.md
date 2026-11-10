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
