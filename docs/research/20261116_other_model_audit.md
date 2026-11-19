# Additional adaptor model review — 2026-11-16

This review covers the 17 adaptor directories below. It compares official public
catalogs, model cards, API schemas and tariffs with the existing gateway contracts.
It is documentation and local regression evidence, not authenticated certification
of every model or an operator's private tariff. Historical IDs remain available;
published account requirements do not become gateway discovery restrictions.

## Changes

### Mistral

[Pricing](https://docs.mistral.ai/inference/pricing), the
[Large 4 card](https://docs.mistral.ai/models/mistral-large-4-0),
[reasoning reference](https://docs.mistral.ai/inference/conversations/reasoning)
and [changelog](https://docs.mistral.ai/resources/changelogs) establish these changes:

- Large 4 is GA. Current input/cache/output prices are $0.70/$0.07/$2.10 per
  million tokens. Supported effort is low/medium/high, default high; the October
  29 change removed none. The exact card aliases include `mistral-large-latest`.
- The two pinned Large 4 IDs keep their existing October 6–20 exclusive UTC
  promotion window. Its timezone remains an explicit gateway convention.
  The latest alias does not inherit that expired promotion from the period when
  it selected Large 3. Pinned `mistral-large-2512` retains its previous tariff.
  No unreported standard-price change date is invented.
- `zai-glm-5-2` now describes the announced October 31 retirement.

Two compatible hosted chat IDs are added with independent metadata:

| API ID | Input / cache / output USD per million | Context / output | Evidence |
| --- | --- | --- | --- |
| `zai-glm-5-3-flash` | 0.15 / 0.03 / 0.50 | 1,000,000 / 128,000 | [Model card](https://docs.mistral.ai/models/zai-glm-5-3-flash) |
| `deepseek-v4-1-flash-1017` | 0.30 / 0.007 / 1.20 | 1,000,000 / 128,000 | [Model card](https://docs.mistral.ai/models/deepseek-v4-flash-1017) |

The DeepSeek card URL omits the `-1` that is present in its API ID. Both models
accept text/images. GLM Flash's documented effort values are retained; no
undocumented DeepSeek string effort vocabulary is inferred.

[Voxtral Mini Transcribe 3](https://docs.mistral.ai/models/voxtral-mini-transcribe-3-0),
[Small Transcribe 3](https://docs.mistral.ai/models/voxtral-small-transcribe-3-0)
and [Mini TTS 3](https://docs.mistral.ai/models/voxtral-mini-tts-3-0) use the existing
offline HTTP audio contracts. Register each card's `-3-0`, `-3` and `-latest`
IDs. Mini/Small transcription cost $0.0035/$0.004 per audio minute; TTS costs
$13 per million input characters. Historical February/March IDs keep their
rates. Realtime WebSocket models and cached-audio accounting are not added.

### Cohere

The [model catalog](https://docs.cohere.com/docs/models),
[Embed reference](https://docs.cohere.com/docs/cohere-embed) and
[Embed 5 announcement](https://cohere.com/blog/embed-5) document `embed-v5.0-pro`
and `embed-v5.0-fast`: 128k context, 256/512/768/1024/1536/2048 dimensions, and
text rates of $0.12/$0.08 per million tokens. Both publish $0.40 per million image
tokens. The existing v2 Embed bridge admits text only; the new discovery metadata
states that restriction, while preserving the published modality tariff. Explicit
768/2048 dimensions now reach Cohere; v3/v4 validation remains model-specific.

The published North IDs contain a dotted version:
[north-mini-code-1.0](https://docs.cohere.com/docs/north-mini-code-1.0) and
[north-small-translate-1.0](https://docs.cohere.com/docs/north-small-translate-1.0).
Add these canonical IDs and retain the historical gateway hyphen spellings as
independently owned configurations. Their documented public free access is not
substituted for a dedicated Model Vault contract.

[Rerank 5](https://cohere.com/blog/rerank-5) changes the tariff to input tokens
($0.05/$0.02 per million for Pro/Fast). The present rerank adapter meters
`search_units` receipts. Its new IDs are therefore deferred until native token
receipts, reservation and settlement are implemented; copying the older per-search
tariff would misbill them. Existing Rerank models remain unchanged.

### Cloudflare Workers AI

Native additions use each Workers AI card's own tariff and API ID:

| ID | Input / cache / output USD per million | Context | Evidence |
| --- | --- | --- | --- |
| `@cf/deepseek-ai/deepseek-v4.1-flash-1017` | 0.30 / 0.006 / 1.20 | 1,048,576 | [Card](https://developers.cloudflare.com/workers-ai/models/deepseek-v4.1-flash-1017/) |
| `@cf/thinkingmachines/inkling-2-small` | 0.30 / 0.10 / 1.00 | 1,048,576 | [Card](https://developers.cloudflare.com/workers-ai/models/inkling-2-small/) |
| `@cf/google/embeddinggemma-2` | 0.0017 / unspecified / n/a | 8,192 | [Card](https://developers.cloudflare.com/workers-ai/models/embeddinggemma-2/) |

Both chat entries preserve their distinct effort vocabulary and default high.
EmbeddingGemma 2 exposes standard text embeddings; its separate nested media
conversation schema is not advertised. No undocumented output limit or precision
is copied from a checkpoint. Workers Paid eligibility remains an upstream decision.

The [Workers AI changelog](https://developers.cloudflare.com/changelog/product/workers-ai/)
moves the announced November 15 retirements to November 20, after which listed
models return HTTP 410/code 5028. Annotate all matching existing catalog IDs,
including the former DeepSeek V4 snapshots and GLM 5.3 Flash, without deleting
them or changing their historical tariffs. New typed-decision endpoints are
outside the chat schema. [Apertus](https://developers.cloudflare.com/workers-ai/models/apertus-v1.5-8b/)
has no published tariff in the retrieved card, so no zero price is invented.

### NVIDIA hosted inference

Nine public free trial chat endpoints are added. Paid Partner Endpoints and
private deployments still require operator pricing. Each linked page's hosted
specifications and embedded request schema were checked together with its
`/modelcard`; card-level checkpoint maxima do not override hosted limits.

| Canonical API ID | Hosted context | Published string effort | Source |
| --- | --- | --- | --- |
| `mistralai/mistral-large-4-1t-a52b` | 262,144 | none/high, default high | [Endpoint](https://build.nvidia.com/mistralai/mistral-large-4-1t-a52b) |
| `nvidia/nemotron-3.5-bolt-e4b` | 65,536 | Boolean native control | [Endpoint](https://build.nvidia.com/nvidia/nemotron-3.5-bolt-e4b) |
| `minimaxai/minimax-m3.1-flash` | 262,144 | Native chat-template controls | [Endpoint](https://build.nvidia.com/minimaxai/minimax-m3.1-flash) |
| `deepseek-ai/deepseek-v4.1-flash` | 1,048,576 | No confirmed string control | [Endpoint](https://build.nvidia.com/deepseek-ai/deepseek-v4.1-flash) |
| `nvidia/nemotron-3.5-super-vl-120b-a12b` | 262,144 | none/low/high, default high | [Endpoint](https://build.nvidia.com/nvidia/nemotron-3.5-super-vl-120b-a12b) |
| `nvidia/nemotron-3.5-lightning-30b-a3b` | 1,048,576 | Boolean/numeric native controls | [Endpoint](https://build.nvidia.com/nvidia/nemotron-3.5-lightning-30b-a3b) |
| `stepfun-ai/step-5-preview` | 1,048,576 | Always enabled | [Endpoint](https://build.nvidia.com/stepfun-ai/step-5-preview) |
| `moonshotai/kimi-k3.1-pro` | 1,048,576 | low/medium/high/max, default low | [Endpoint](https://build.nvidia.com/moonshotai/kimi-k3_1-pro) |
| `meta/muse-glimmer-30b` | 131,072 | Inconsistent schema; not inferred | [Endpoint](https://build.nvidia.com/meta/muse-glimmer-30b) |

Mistral's NVIDIA schema still specifies none/high despite the first-party
Mistral service changing its vocabulary. Kimi's API ID uses a dot where its URL
uses an underscore. Muse's embedded effort description names Inkling, so it is
not used to certify Muse controls. Step and Muse explicitly lack structured
output. The converter now retains `reasoning_effort` for catalog models with a
documented vocabulary, instead of a hard-coded two-GLM allowlist. Existing
omission behavior remains for other models; native `extra_body` data survives
for the shared controlled-passthrough layer.

### TypeSafe

The [official model reference](https://docs.typesafe.ai/models) now assigns both
`jev-latest` and `jev-preview` to `jev-1.14.0`. Update descriptive metadata and
the adaptor README; preserve pinned 1.13/1.14 IDs, $0.042/M input, free output,
64k aggregate context, and native System One behavior. Optional image/extended
context features are not inferred from the catalog update.

### Replicate

The [Gemini 3.1 Pro tariff](https://replicate.com/google/gemini-3.1-pro) specifies
$2/$12 input/output per million through 200,000 input tokens, then $4/$18.
The code previously described but did not encode that second tier. Add the
inclusive threshold 200,001 and test both sides through the pricing resolver.
Remove unconfirmed GPT-5.6 context/output limits copied from a sibling model;
the [Sol](https://replicate.com/openai/gpt-5.6-sol),
[Terra](https://replicate.com/openai/gpt-5.6-terra) and
[Luna](https://replicate.com/openai/gpt-5.6-luna) pages do not establish those
limits. Preserve all existing prices and Sol's expired September promotion.

[Gemini 4 Carbon](https://replicate.com/google/gemini-4-carbon/readme) is a new
candidate, with a documented introductory tariff through November 30. It is
deferred because its `max_output_tokens`/thinking contract and billed reasoning
tokens are not handled by the present generic prompt bridge and estimated text
usage. No image/video entry is moved into a token-priced chat path. This audit
does not claim that every historical Replicate model has been recertified.

## Reviewed without new compiled catalog entries

| Directory | Disposition and official evidence |
| --- | --- |
| `cerebras` | [Public models API](https://api.cerebras.ai/public/v1/models) and [overview](https://inference-docs.cerebras.ai/models/overview) match the existing shared GPT-OSS 120B and Qwen 3.8 defaults. Dedicated/historical entries remain distinct. No creator price is copied. |
| `groq` | [Current models](https://console.groq.com/docs/models) matches the existing production/preview GPT-OSS, Qwen 3.8, Whisper, Orpheus and guard entries. Enterprise availability does not establish a public per-token tariff; existing historical IDs remain. |
| `jina` | [Public models](https://api.jina.ai/v1/models) and [OpenAPI](https://api.jina.ai/openapi.json) retain the native embedding/rerank/OCR roster and prices. VLM/ReaderLM entries outside those discriminators are not added to an incompatible endpoint. Asynchronous embedding batches remain a separate protocol. |
| `deepl` | [API plans](https://support.deepl.com/hc/en-us/articles/360021200939-DeepL-API-plans) and [usage billing](https://support.deepl.com/hc/en-us/articles/360020685720-Usage-count-and-billing-in-DeepL-API) confirm source-character billing and contract-dependent plans. The three `deepl-*` IDs are language aliases, not foundation models. Retain the historical $27.50/M-character default; the rendered [pricing page](https://www.deepl.com/en/pro) did not revalidate that regional amount. Clarify this evidence limitation and Enterprise overrides in source comments. |
| `ollama` | [GET /api/tags](https://docs.ollama.com/api/tags) describes an installation's model inventory and quantization. Preserve the curated compatibility list and historical symbolic local charges. Public library availability cannot establish that a tag is installed on a configured channel. |
| `palm` | Retain the explicitly historical `PaLM-2` alias. The former [PaLM documentation](https://ai.google.dev/palm_docs/palm) and deprecation page returned 404 during this review; they do not support a new current price or broader catalog. Gemini uses its separate adaptor. |
| `coze` | Requests resolve configured bot IDs in the existing adaptor. The public [premium page](https://www.coze.com/premium) did not expose a machine-readable model tariff in the retrieved content. Preserve the clearly labeled compatibility default; do not equate subscription credits with token USD. |
| `copilot` | [Supported models](https://docs.github.com/en/copilot/reference/ai-models/supported-models) varies by client, plan and policy and documents credit consumption. Preserve the existing channel-configured model behavior; display names do not provide a stable public inference ID/tariff mapping for this relay. |
| `muapi` | [Catalog](https://muapi.ai/docs/models) and [pricing](https://muapi.ai/docs/pricing) expose a changing model registry, per-request quotes and cost receipts. Preserve the existing dynamic video quote/settlement path and acceptance of unlisted valid slugs; no frozen token table is introduced. New chat/media protocols are outside that video contract. |
| `proxy` | Protocol adaptor with channel-configured upstream models; no independent provider tariff or universal compiled model catalog. Existing forwarding remains unchanged. |
| `openai_compatible` | Shared protocol conversion and response utilities, not a provider catalog. Existing model preservation and conversion behavior remains unchanged. |

## Validation

Using the installed Go 1.27.4 toolchain, all of the following passed:

```text
go test ./relay/adaptor/{mistral,cohere,groq,cerebras,cloudflare,nvidia,jina,deepl,typesafe,ollama,palm,coze,copilot,proxy,muapi,replicate,openai_compatible}
go test ./relay/controller -run '^TestProtocolAuditAudioHTTP/mistral_latest$' -count=1
```

The focused regressions cover provider tariffs and cache prices, explicit effort
serialization, canonical IDs, embedding dimensions, independent nested metadata,
promotion boundaries, the Replicate long-context threshold, and the changed TTS
latest-alias charge through the real controller/quota fixture. No authenticated
provider generation was performed. Repository-wide checks are recorded separately
by the coordinating PR review.
