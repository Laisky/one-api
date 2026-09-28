# Claude model catalog review — 2026-09-28

Base reviewed: `b1a2273ef41d4375ffe7bc5f18888ce11356285c` (`main`). The cloud-provider follow-up builds on PR #431 head `46f9a201960492569a8dfbd21f86d738ded779b9` and retains its native Anthropic and Vertex additions.

## Scope and release gates

This is a catalog, pricing-metadata, and Bedrock profile-registration update, **not complete Sonnet 5.5 migration support**. Definitions are directly editable Go code, following `relay/adaptor/MODELS.md`; no generator, runtime catalog fetch, database migration, account probe, or extra inference attempt is introduced. Existing model definitions and operator settings are not replaced.

**Before merge/deployment:** obtain passing CI for the final head and independently confirm the new Bedrock tariff (or explicitly approve/configure the reference defaults). The new AWS prices are a documented inference, not a retrieved AWS price quote. Restricted Foundry models still need appropriate authentication. The request-compatibility gaps below remain open.

## Provider-by-provider disposition

| Surface | Result | Evidence and boundary |
| --- | --- | --- |
| Anthropic | Add `claude-sonnet-5-5`; retain the prior eight-model pricing regression table. | Official model overview and first-party price table. No manufactured snapshot or `-latest` IDs. |
| Amazon Bedrock | Add the public alias, `anthropic.claude-sonnet-5-5` mapping, independent capability definition, and `global.anthropic.claude-sonnet-5-5` registration for 33 documented commercial source regions. | AWS model card independently establishes ID, limits, Invoke/streaming support, global routing, and capability exclusions. **Pricing is a launch reference inference requiring confirmation**, as explained below. |
| Microsoft Foundry / Azure | Add five missing discoverable IDs: Sonnet 5.5, Opus 5.5, Opus 5, Fable 5.1, Mythos 5.1. | Microsoft/Anthropic Foundry model documentation; Foundry's published pricing uses the first-party per-model rates. Reuse existing Claude-specific Azure routing and pricing, not the Azure OpenAI deployment endpoint. |
| Google Cloud / Vertex AI | Retain the independently defined Sonnet 5.5 entry and parent-registry discovery/URL tests. | Google global pricing and the published undated model ID. Regional premiums remain channel configuration. |
| OpenRouter | Add `anthropic/claude-sonnet-5.5` with OpenRouter's own pricing, limits, reasoning efforts, and supported-parameter list. | Public `/api/v1/models` JSON, not the creator's catalog. Do not use `canonical_slug` as a separate public ID or add the unqualified asynchronous `:batch` variant. |
| GitHub Copilot | Reviewed; no static catalog change. | Copilot's pricing page now lists Sonnet 5.5, but friendly names do not establish the wire IDs, plan-specific limits, or the subscription settlement contract. This repository's Copilot adaptor has no static model list and rejects Claude Messages input. Preserve operator configuration rather than claim complete support. |
| Claude Platform on AWS | Reviewed separately from Bedrock; no new channel/authentication implementation. | Anthropic documents first-party prices for its own AWS-hosted service. That statement is **not** proof of partner-operated Bedrock pricing or compatibility with the existing Bedrock SDK transport. |

The scope is these Claude-bearing surfaces, not a new audit of unrelated GPT, Gemini, image, or open-weight models. A discoverable model is not evidence of account entitlement, regional availability, or successful production inference.

## Standard token prices

USD per million tokens. These are the verified first-party rates and the existing native public-method regression oracle, excluding Batch, fast mode, residency premiums, and private contracts.

| Model ID | Input | Output | Cache read | 5m write | 1h write |
| --- | ---: | ---: | ---: | ---: | ---: |
| `claude-sonnet-5-5` | 2 | 10 | 0.20 | 2.50 | 4 |
| `claude-sonnet-5` | 2 | 10 | 0.20 | 2.50 | 4 |
| `claude-opus-5-5` | 4 | 20 | 0.20 | 5 | 8 |
| `claude-opus-5` | 5 | 25 | 0.50 | 6.25 | 10 |
| `claude-fable-5-1` | 10 | 50 | 0.25 | 12.50 | 20 |
| `claude-mythos-5-1` | 10 | 50 | 0.25 | 12.50 | 20 |
| `claude-fable-5` | 10 | 50 | 1 | 12.50 | 20 |
| `claude-mythos-5` | 10 | 50 | 1 | 12.50 | 20 |

Do not apply a universal 10% cache-read multiplier: Opus 5.5 uses 5%, while Fable/Mythos 5.1 use 2.5%. Do not restore Sonnet 5's expired introductory-price window. Foundry's published Claude Compute Unit billing uses the same underlying per-model rates. Google global and OpenRouter Sonnet 5.5 prices were separately checked against their own sources.

Source: [Anthropic pricing, including cloud-platform distinctions and Foundry billing](https://platform.claude.com/docs/en/about-claude/pricing).

### Bedrock pricing qualification

The AWS model card identifies the Marketplace product and links to Bedrock pricing, but the retrieved AWS pricing page did not expose Sonnet 5.5's dynamic tariff table. Therefore this change does **not** claim independent AWS confirmation of the five price buckets.

The new default is $2 input / $10 output / $0.20 read / $2.50 five-minute write / $4 one-hour write. It follows Anthropic's explicit statement that Sonnet 5.5 retains Sonnet 5 prices, including caching, together with the repository's existing Bedrock Sonnet 5 defaults. Applying that parity to Bedrock is an **inference**. Source comments, the displayed description, and the tests label it as a reference default; assertions establish the configured arithmetic, not the upstream invoice. No existing Bedrock rate is changed and no missing value is treated as free.

Independently check the applicable AWS Marketplace agreement before relying on these defaults. A private contract, region-specific rate, or different service tier requires channel overrides. Do not confuse this evidence gap with lack of model availability or lack of Invoke support.

## Limits, routing, and capabilities

Sonnet 5.5 has a 1,000,000-token context and 128,000-token ordinary Messages output limit. Anthropic's 300K output beta is Batch-only and is not advertised as the synchronous limit. Thinking defaults to adaptive/high. No manual reasoning-budget maximum is inferred. The native catalog excludes `temperature`, `top_p`, and `top_k`.

Bedrock's new model card explicitly supports `InvokeModel` and streaming on `bedrock-runtime`. The existing transport remains appropriate. Commercial-region calls select the documented global inference profile, while explicit channel inference-profile ARNs retain precedence. Only the documented global ID is added: no guessed geographic variants, account-availability probes, or fallback retries. The 33-region regression oracle is independently listed in the test. GovCloud, China, and unknown regions are not redirected to the commercial global profile; the existing resolver's raw-ID fallback is retained there and does not establish supported inference. Global inference does not provide single-region data residency.

The Bedrock launch card excludes native structured outputs and Batch. Its definition therefore advertises only the common client-tool/reasoning core, not first-party server features. Text/image/document inputs follow the existing `file` metadata convention; that does not imply a Files API exists on every provider.

Azure already dispatches Claude to `/anthropic/v1/messages`, using the mapped deployment name. New tests cover URL/header selection for Chat, Messages, and Responses request paths, plus real Azure-to-Claude Chat conversion. These tests do not certify the entire Responses bridge or model-specific normalization for arbitrary deployment aliases. Microsoft requires Entra ID for the gated Mythos family; adding catalog IDs does not implement Entra token acquisition or refresh and does not make API-key access to those models valid.

Vertex retains global standard rates and its conservative `tools`/`reasoning` profile. Regional and multi-region pricing premiums require channel overrides. OpenRouter retains its **own** advertised `temperature` parameter; its public API contract must not be replaced with Anthropic's native allowlist. OpenRouter's new family constructor returns independently owned slices and is assembled with the existing duplicate-rejecting `JoinModelCatalogs` helper.

## Existing migration gaps — not fixed here

Sonnet 5.5 adds `thinking.type=between_tools`, rejects forced tool choices, and changes thinking-block compatibility. The current typed `NormalizeModelCompatibility` and native raw `rewriteClaudeAdaptiveThinking` both coerce non-adaptive thinking to `adaptive`. The Chat converter also applies a manual-thinking minimum-output guard to non-adaptive types. Do not rely on this catalog PR to preserve `between_tools`, faithfully migrate `disabled`, or validate all forced-tool combinations.

A runtime follow-up must cover typed Chat and native raw Messages together, preserve supported small output limits, signatures, exact integers, caller effort, and unknown extensions, and test the actual final upstream body. No one-path prototype or silent change to forced tool choice is included here.

## Regression coverage and validation

The original native/Vertex tests remain. The follow-up adds public catalog/rate/capability checks for Bedrock, five Azure entries, and OpenRouter; all 33 global source-region mappings and unsupported-region controls; eight real-AWS-SDK fixture combinations (native/converted x JSON/streaming x global/explicit ARN); cumulative cache-usage preservation; mapped Azure deployment routing and API-key headers; canonical Azure Chat conversion; invented-ID exclusions; and independently owned OpenRouter metadata.

Local checks: Go formatting, parser validation, function documentation, and file-length checks for the ten follow-up Go files. The two modified existing files were reconstructed and verified against their exact original Git blob hashes before editing. **Repository tests, race tests, vet, and build were not executed locally:** the editor has Go 1.23.2, while the repository requires Go 1.27.1 and toolchain/dependency downloads are unavailable. New tests are not reported as passing until CI actually runs them. Original-head CI results do not qualify the follow-up head.

Required validation with the declared toolchain:

```sh
go test -race ./relay/adaptor/anthropic ./relay/adaptor/aws/... ./relay/adaptor/azure ./relay/adaptor/vertexai/... ./relay/adaptor/openrouter ./relay
go vet ./...
go test -race ./...
go build -o one-api .
```

No paid provider requests, live account-availability probes, production deployment, automatic merge, or provider-invoice reconciliation was performed.

## Official sources

- [Sonnet 5.5 overview: IDs, limits, and launch](https://platform.claude.com/docs/en/models/sonnet-5-5/overview)
- [Sonnet 5.5 price-parity statement](https://platform.claude.com/docs/en/models/sonnet-5-5/whats-new-sonnet-5-5#pricing)
- [Sonnet 5.5 migration guide](https://platform.claude.com/docs/en/models/sonnet-5-5/migration-guide)
- [AWS Sonnet 5.5 model card](https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-sonnet-5-5.html)
- [AWS Bedrock pricing — dynamic model tariff not retrieved](https://aws.amazon.com/bedrock/pricing/)
- [Microsoft Foundry Claude catalog](https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/claude-models)
- [Claude in Microsoft Foundry: endpoints, deployment names, authentication](https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry)
- [Google Cloud pricing](https://cloud.google.com/vertex-ai/generative-ai/pricing)
- [Claude on Vertex AI](https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai)
- [OpenRouter public models JSON](https://openrouter.ai/api/v1/models)
- [Copilot model pricing](https://docs.github.com/en/copilot/reference/copilot-billing/models-and-pricing)
