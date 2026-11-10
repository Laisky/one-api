# Anthropic model refresh — November 10, 2026

## Scope and sources

Add Claude Fable 5.2 and Claude Mythos 5.2, released November 9, to the
native Anthropic, Amazon Bedrock, Google-managed Vertex AI and Microsoft
Foundry adaptors. All existing model IDs and administrator pricing overrides
are preserved. Model discovery does not imply provider account entitlement.

Official documentation:

- https://platform.claude.com/docs/en/models/fable-5-2/overview
- https://platform.claude.com/docs/en/models/mythos-5-2/overview
- https://platform.claude.com/docs/en/models/fable-5-2/whats-new-fable-5-2
- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-fable-5-2.html
- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-mythos-5-2.html
- https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/claude-models

## Model metadata and standard tariffs

Both have a 1M-token context, 128K ordinary output limit, image/text input,
text output, always-on adaptive thinking and high default effort. The published
undated API IDs are `claude-fable-5-2` and `claude-mythos-5-2`.

Anthropic standard USD per million tokens for both: **$8 input**, **$40 output**,
**$0.20 cache read**, **$10 five-minute cache write**, **$16 one-hour cache write**.
Fast mode doubles input/output rates and Batch discounts them; this change
does **not** enable special-mode billing. No made-up dated or latest aliases.

## Platform-specific constraints

- Anthropic: existing Fable/Mythos adaptive-thinking compatibility prefixes
  already cover 5.2. Preserve native Claude Messages blocks and signatures.
- AWS Bedrock: public model names map to `anthropic.claude-*-5-2`, then to
  published Global inference profiles for supported commercial regions. The
  US geo profile is also registered. In-region Invoke is unavailable. GovCloud
  has no global routing. Explicit inference profile ARNs remain authoritative.
  AWS Fable 5.2 requires an AWS data-retention opt-in (`aws_review`).
- Vertex AI: use Google-managed IDs and the conservative tools/reasoning
  feature subset. The retrieved Google pricing page did not establish the 5.2
  tariffs. The public Vertex adaptor therefore exposes no default prices for
  these two IDs and requires explicit channel input/output and all cache tariffs
  before dispatch. Existing administrator settings are read, never rewritten.
  First-party numeric values retained in sub-adaptor metadata are references,
  not verified Google prices. Global, US multi-region and EU multi-region are
  documented locations; endpoint-specific Google prices must be checked.
- Microsoft Foundry: Anthropic-hosted preview; Mythos is restricted and
  requires Entra ID. Foundry supports low/medium/high/xhigh, **not** max.
  Neither separate fast deployment is advertised because its premium billing
  has not been implemented.

The new models have model- and conversation-prefix-bound thinking blocks.
Users must replay preserved thinking unchanged. No new transformations of
opaque content or provider entitlements are introduced.

## Tests and qualification

Focused Go regressions cover discovery, full cache tariffs, legacy pricing
retention, AWS model-ID dispatch and inference profiles, Vertex metadata, and
the Foundry effort differences.

```sh
go test ./relay/adaptor/anthropic ./relay/adaptor/aws/... ./relay/adaptor/vertexai/... ./relay/adaptor/azure
make test-quick
```

Live inference and full race qualification remain separate manual checks.

## Review qualification and operator prerequisites

Checked official documentation on November 10, 2026; no live inference or
provider-account configuration changes were performed.

- Native Anthropic: both 5.2 models require 30-day retention unless expressly
  exempted, and Mythos requires verified access. Retention/access failures must
  remain visible; do not change data sharing or substitute a model automatically.
- Bedrock: Fable requires operator-configured `aws_review`; Mythos requires
  `aws_review` or `provider_data_share`. These are account prerequisites,
  not settings for the gateway to silently enable.
- Google-managed Fable: model enablement, safety/Marketplace terms, applicable
  prompt-response sharing and IAM must be completed by an authorized operator.
  Google documents up-to-30-day abuse-monitoring retention. Catalogue inclusion
  does not establish these prerequisites.
- Foundry: the standard 5.2 models are Anthropic-hosted previews; Mythos is
  gated and Entra-ID-only. Effort `max` is not supported. Fast variants are
  separate deployments and remain excluded from the default catalogue.
- Preserve signed thinking blocks, empty thinking blocks and redacted content
  unchanged. Earlier system/tools/history edits can invalidate signatures.
  Local preservation tests cannot establish provider-side signature acceptance.

Vertex reuses the final Anthropic body validator. Unpriced fast-mode requests
are rejected, not silently downgraded. Google documents a 2x premium across
input, output and cache token categories; first-party multipliers must not be
assumed. Tests cover public dispatch rejection before authentication/network,
complete custom tariffs, missing components, tiers, mapped names, and unchanged
legacy defaults. Operator-owned complete tariffs are a necessary admission
condition, not proof that the configured amounts match a provider invoice.

Additional sources:
- https://platform.claude.com/docs/en/models/fable-5-2/migration-guide
- https://platform.claude.com/docs/en/build-with-claude/claude-models-on-google-cloud/google-managed
- https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/claude/fable-5-2
- https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/claude/fast-mode
- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-mythos-5-2.html

Manual acceptance before production traffic: confirm each account's access,
retention and selected endpoint; verify standard and cache rates against the
current provider contract; replay unchanged tool-loop history; check both
streaming and unary usage receipts and expected authorization failures. Live
qualification remains pending and must not be inferred from local test results.

## Release qualification checklist (review C)

Every item below is **pending live operator qualification**. Local fixture tests
establish gateway behavior only. For each provider/model combination, record the
operator, UTC observation time, exact gateway commit, account/project/deployment
identifier (redacted as needed), model ID, endpoint/source region, approved
routing scope, and a link to access-controlled evidence. Never store credentials,
full prompts, signed thinking content, or sensitive response payloads in this
repository or task history.

- [ ] Entitlement: confirm the selected account can invoke the exact model.
  For Mythos, verify organization/account approval rather than inferring it from
  model discovery. Check a denied-access control remains denied without a model
  substitution, alternate endpoint, or automatic retry into another region.
- [ ] Credentials and authorization: confirm the intended runtime identity,
  provider-specific authentication and least-privilege invocation permissions.
  Foundry Mythos requires Entra ID; Google-managed Vertex requires the selected
  project's IAM/model enablement; Bedrock requires permissions on the selected
  model/profile. Record identity metadata only, never key/token material.
- [ ] Retention and consent: have the authorized operator verify current provider
  terms, exemptions and required sharing/retention settings described above.
  The gateway must not enable these settings or accept terms as a side effect.
  Missing consent is a deployment blocker, not a reason to change providers.
- [ ] Region and routing: verify the source region and actual permitted
  destination scope of the configured endpoint/profile, including organization
  policies. A successful global call does not prove single-region residency.
- [ ] Price contract: verify standard input/output, cache reads, five-minute
  writes, one-hour writes, tiers and time windows against the selected provider
  contract. For Vertex, configure the complete verified tariff explicitly;
  satisfying the configuration validator is not invoice verification.
- [ ] Token receipts: run separately authorized minimal unary and streaming
  calls, with ordinary input/output and applicable cache read/write cases.
  Record redacted request/receipt IDs and numeric usage counters; reconcile
  upstream counters, gateway token/quota charges, user/token balance changes
  and cost/log records. Do not substitute text-length estimates for receipts.
- [ ] Failure accounting: confirm admission rejection sends no upstream request
  and no charge; after an upstream paid receipt exists, delivery failures retain
  the receipt and settle exactly once. Test authorization failure, cancellation
  and streaming termination without silently rerouting or losing accounting.
- [ ] Conversation compatibility: replay an approved tool loop with unchanged
  opaque/empty/redacted thinking blocks and model/history binding. A local JSON
  preservation assertion does not establish provider acceptance of signatures.

Any live calls incur provider usage and require separate authorization. This
checklist does not authorize purchases, production traffic, credentials,
consent changes or a release. Keep release readiness unconfirmed until the
applicable evidence and rollback decision have been reviewed.

## Cross-region routing and residency (review D)

For these two Bedrock models, the default resolver chooses the documented
**global** inference profile from supported commercial source regions, including
Canadian and European source regions. The configured client/source region is
not a promise that processing stays in that region or country. The registered
US geo profile is not the automatic default. No EU profile, GovCloud-global
profile or raw in-region invocation is fabricated by this change.

An explicitly configured channel inference-profile ARN takes precedence over
catalog mapping and the automatic global choice. Administrators with residency
constraints must provision and verify a suitable supported profile/endpoint and
its permitted destination set, IAM/SCP policies and model entitlement before
traffic. An ARN override is routing authority, not evidence that its profile
meets a residency requirement or is valid in every partition. If no supported
choice meets the requirement, leave the model disabled; do not fall back to a
global profile to make a request succeed.

Unsupported raw 5.2 routing fails before SDK invocation. Authorization errors,
throttling, timeout or malformed payload responses never authorize an automatic
switch to a different profile, model or geography. Explicit overrides and
legacy-model routes are covered by behavioral regression tests; provider-side
geographic guarantees remain an operator/provider qualification responsibility.

## GovCloud and routing-price follow-up — November 10

AWS announced Fable 5.2 availability in GovCloud on November 9. The Fable and
Mythos model cards each list us-gov-east-1 and us-gov-west-1 Geo support, but not
Global or in-region invocation. Therefore the current automatic-route rejection
means **this gateway's GovCloud profile mapping is not yet qualified**, not that
the provider model is unavailable.

The retrieved detailed profile tables did not establish GovCloud profile IDs
and destination sets; they direct the operator to GetInferenceProfile in the
source region. No actual GovCloud profile API receipt has been obtained in this
task. A bounded dev SDK check found boto3/botocore 1.34.46, no default
credential identity or configured profiles, and no inference-profile operations
in that SDK. Calls failed locally before any AWS request; this is not evidence
of a provider permission denial or model unavailability. Do not guess a commercial us.* profile is valid in GovCloud, invent a
partition-specific ID, or reuse a commercial Global profile. An authorized
operator must verify the exact profile ARN/ID, source region, destination
models/regions and entitlement before configuring the existing explicit override.
Mythos must be qualified independently rather than inheriting Fable's receipt.

AWS documents differences between Global and Geo routing prices. Exact example
Geo prices of USD8.80/USD44 were not verified from an official retrieved table
and are not introduced as defaults. Explicit non-Global/custom routing requires
operator-verified tariffs; neither an ARN nor successful profile lookup proves
price parity. Stored administrator rates must not be rewritten automatically.

Sources:
- https://aws.amazon.com/about-aws/whats-new/2026/11/claude-fable-5-2-aws-govcloud/
- https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-fable-5-2.html
- https://docs.aws.amazon.com/us_en/bedrock/latest/userguide/model-card-anthropic-claude-mythos-5-2.html
- https://docs.aws.amazon.com/bedrock/latest/userguide/models-region-compatibility.html
- https://aws.amazon.com/bedrock/pricing/

## Separate acceptance claims

1. Catalog support: exact published identities, feature/effort metadata and
   legacy entries must pass catalog/identity regressions. This proves discovery
   and matching, not provider entitlement or price contracts.
2. Provider admission and dispatch: fixtures must exercise production client
   construction, one physical attempt per generation, forwarding evidence,
   explicit routing/pricing admission and terminal provider errors. A test-only
   retryer is not evidence about the production default.
3. Accounting: finite-balance controller tests must reconcile balances,
   provisional logs and request cost exactly once. Recovery tests must inject
   separate accounting-store failures and protect later successful retries.
4. Live enablement: the operator checklist above remains separate. Local
   transport fixtures and database tests do not establish live provider access,
   residency, negotiated prices or real usage receipts.
