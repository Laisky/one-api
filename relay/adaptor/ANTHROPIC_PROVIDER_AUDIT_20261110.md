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
- Vertex AI: use the Google-managed IDs, standard global pricing and the
  conservative tools/reasoning feature subset. Regional prices may differ.
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
