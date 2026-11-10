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

Standard USD per million tokens for both: **$8 input**, **$40 output**,
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
