# Anthropic model catalog review — 2026-09-28

Base reviewed: `b1a2273ef41d4375ffe7bc5f18888ce11356285c` (`main`).

## Delivered scope

Add the published `claude-sonnet-5-5` ID to the Anthropic and Vertex AI catalogs. The model was released on September 28, 2026. Both providers publish this undated ID; do not manufacture dated snapshots, `-latest` aliases, or Vertex `@date` suffixes.

Use direct Go `ModelConfig` definitions, consistent with `relay/adaptor/MODELS.md`. Refresh the Vertex child model list so the parent registry discovers the entry. Existing models, billing code, configured channels, and operator overrides are unchanged. A catalog entry does not prove an account has model access.

## Verified standard token prices

USD per million tokens. These are standard first-party prices, not Batch, fast-mode, data-residency, or negotiated prices.

| Model ID | Input | Output | Cache read | 5m write | 1h write | Change |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| `claude-sonnet-5-5` | 2 | 10 | 0.20 | 2.50 | 4 | Added |
| `claude-sonnet-5` | 2 | 10 | 0.20 | 2.50 | 4 | Retained |
| `claude-opus-5-5` | 4 | 20 | 0.20 | 5 | 8 | Retained |
| `claude-opus-5` | 5 | 25 | 0.50 | 6.25 | 10 | Retained |
| `claude-fable-5-1` | 10 | 50 | 0.25 | 12.50 | 20 | Retained |
| `claude-mythos-5-1` | 10 | 50 | 0.25 | 12.50 | 20 | Retained; restricted access |
| `claude-fable-5` | 10 | 50 | 1 | 12.50 | 20 | Retained |
| `claude-mythos-5` | 10 | 50 | 1 | 12.50 | 20 | Retained; restricted access |

The public-method regression table covers these rates. Do not apply a universal 10% cache-read multiplier: Opus 5.5 uses 5%, while Fable/Mythos 5.1 use 2.5%. Sonnet 5's permanent price must not revert to an expired introductory-price window.

Source: [Anthropic pricing](https://platform.claude.com/docs/en/about-claude/pricing).

## Sonnet 5.5 limits and provider boundaries

The ordinary Messages limit is **1,000,000 context tokens and 128,000 output tokens**. The 300K output limit belongs to a Batch-only beta and is not the synchronous default. Adaptive thinking defaults to high effort; no manual reasoning-budget maximum is advertised. The catalog excludes `temperature`, `top_p`, and `top_k`, matching the existing adaptive-only compatibility profile. Text/image/document inputs and text outputs follow the existing Claude catalog conventions; `file` is not a claim that every platform implements the Files API.

Google's own pricing table independently lists the same Sonnet 5.5 **global** rates. Regional and multi-region rates have a 10% premium and require channel pricing overrides. The Vertex entry retains the converter's conservative `tools`/`reasoning` feature profile rather than inheriting first-party server tools. This does not claim the upstream Google service lacks other features.

The published Bedrock ID is `anthropic.claude-sonnet-5-5`. However, the AWS pricing page retrieved during this review did not expose a verifiable Sonnet 5.5 tariff. **No AWS Sonnet 5.5 default is added in this PR.** Do not copy first-party prices into a separately billed provider without evidence. Existing Bedrock entries remain unchanged. This is an evidence limitation, not a claim that Bedrock lacks the model or that its Invoke APIs cannot reach it.

Sources:

- [Sonnet 5.5 overview, IDs, limits and launch date](https://platform.claude.com/docs/en/models/sonnet-5-5/overview)
- [Google Cloud pricing](https://cloud.google.com/vertex-ai/generative-ai/pricing)
- [Claude on Vertex AI](https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai)
- [Amazon Bedrock pricing](https://aws.amazon.com/bedrock/pricing/)

## Known migration gaps — not fixed by this catalog PR

**This is not full Sonnet 5.5 migration support.** The provider introduces `thinking.type=between_tools`, rejects forced tool choice, and changes thinking-block compatibility. See the [migration guide](https://platform.claude.com/docs/en/models/sonnet-5-5/migration-guide).

On the reviewed base, `NormalizeModelCompatibility` in `relay/adaptor/anthropic/compat.go` and `rewriteClaudeAdaptiveThinking` in `relay/controller/claude_messages_request.go` coerce non-adaptive thinking types to `adaptive`. The Chat converter in `relay/adaptor/anthropic/main.go` also applies a manual-thinking output minimum to non-adaptive types. Consequently, **do not rely on `between_tools` being preserved**. Changing only the catalog or one normalization path does not fix this.

A complete follow-up must test typed Chat conversion and native raw Messages rewriting together; preserve valid `between_tools` requests at small output limits; preserve signatures and exact integers; and reject unsupported combinations without silently changing forced tool choice or effort. The existing legacy normalization of `disabled` is not certified as a faithful Sonnet 5.5 migration either. Those runtime changes are deliberately excluded rather than shipping a partial fix.

## Validation

Added repository tests cover eight model price profiles, new-model discovery and limits, rejection of invented aliases, default/adaptive Chat wire conversion, Vertex child-adaptor selection, global request URL construction, and provider-specific feature isolation. They perform no paid inference requests.

Local checks: `gofmt` and Go parser validation of all four changed Go files. **Repository tests, race tests, vet and build were not executed locally**: the editor has Go 1.23.2, the repository requires Go 1.27.1, and network/toolchain acquisition is unavailable.

Required repository validation with the declared toolchain:

```sh
go test ./relay/adaptor/anthropic ./relay/adaptor/vertexai/... ./relay
go test -race ./relay/adaptor/anthropic ./relay/adaptor/vertexai/...
go test ./...
go vet ./...
go build -o one-api .
```

A queued CI run is not a passing result. No live-provider acceptance or account-level availability has been established.
