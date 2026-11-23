# Additional adaptor model review — 2026-11-22

This is a follow-up to the November 16 review. It checks current official
catalogs, release notes, public model APIs and the gateway's implemented
transports. A review disposition does not certify authenticated inference or
private account pricing. Existing configured IDs and historical tariffs remain
available. Source access limitations are recorded explicitly.

## xAI lifecycle correction

The [official release notes](https://docs.x.ai/developers/release-notes) state
that `grok-build-0.1` reached end of life on November 19, 2026 and is no longer
served. Its description now records that status. The historical
`grok-code-fast-1`, `grok-code-fast`, and `grok-code-fast-1-0825` entries describe
the retirement of their documented target without inventing a replacement
route. Their IDs, prices and request forwarding remain unchanged. The separate
`grok-build-latest` alias continues to describe its existing Grok 4.5 target.

The [Grok 4.7 card](https://docs.x.ai/developers/models/grok-4.7) agrees with the
existing 500,000-token context, text/image inputs, tools, structured outputs,
low/medium/high/xhigh efforts and default high. Standard USD/M input/cache/output
remains 2/0.5/6, with the documented higher-context tariff already present.
The [speech-to-text page](https://docs.x.ai/developers/models/speech-to-text)
describes a separate native voice contract; it is not registered as a chat model.

## Reviewed without further compiled model changes

| Directory | Evidence checked and disposition |
| --- | --- |
| `cerebras` | [Public models API](https://api.cerebras.ai/public/v1/models) and [overview](https://inference-docs.cerebras.ai/models/overview): GPT-OSS 120B remains USD 0.35/0.75 per million input/output tokens with 131,072 context and 40,960 output. Qwen 3.8 27B remains 0.99/1.49, with live API limits 65,536/32,768. The overview's larger paid-context claim remains explicitly distinguished from the public API. Dedicated compatibility entries are retained. |
| `groq` | [Supported models](https://console.groq.com/docs/models): public GPT-OSS 120B/20B, Qwen 3.8, Whisper, Orpheus and guard models match the existing roster and listed rates/limits. Enterprise entries still say Contact Sales; no public price is substituted for a private contract. |
| `mistral` | [Standard pricing](https://docs.mistral.ai/inference/pricing), [Large 4 card](https://docs.mistral.ai/models/mistral-large-latest), and [changelog](https://docs.mistral.ai/resources/changelogs): Large 4 remains USD 0.70/0.07/2.10 per million input/cache/output tokens; hosted GLM/DeepSeek and Voxtral 3 entries are already present. Cached-audio rates and realtime voice remain outside the implemented ordinary audio billing contract. |
| `cohere` | [Model catalog](https://docs.cohere.com/docs/models): current Command, North and Embed 5 entries are already present. Rerank 5's input-token billing still differs from the existing `search_units` settlement path; no older per-search tariff is copied onto it. |
| `cloudflare` | [Workers AI changelog](https://developers.cloudflare.com/changelog/product/workers-ai/): the latest additions exposed in the retrieved changelog are EmbeddingGemma 2 and Inkling 2 Small, already present. The existing November 20 retirement notices remain visible with legacy IDs and historical tariffs. No live request is claimed to verify the announced shutdowns. |
| `nvidia` | [Hosted catalog](https://build.nvidia.com/models), [Kimi model card](https://build.nvidia.com/moonshotai/kimi-k3_1-pro/modelcard), and [Fastino page](https://build.nvidia.com/fastino): current compatible chat additions are already present. The new GLiNER2 PII entity-extraction endpoint and protein-design endpoints do not establish a Chat Completions contract. Hosted limits and free trial conditions are not replaced by creator maxima or commercial deployment prices. |
| `jina` | [Public model API](https://api.jina.ai/v1/models) and [OpenAPI](https://api.jina.ai/openapi.json), version `2026.10.13.1624`: the existing native embedding/rerank/OCR inventory remains applicable. The schema also has asynchronous batch endpoints, which are distinct from these synchronous adaptor paths. No public-catalog ReaderLM/VLM entry is promoted into an unsupported request schema. |
| `typesafe` | [Official models reference](https://docs.typesafe.ai/models): both mutable Jev aliases still point to `jev-1.14.0`; pinned 1.13/1.14 defaults, USD 0.042/M input and free output remain correct. Optional image, Box-question and extended-context contracts are not inferred from the model name. |
| `replicate` | [Gemini 3.1 Pro](https://replicate.com/google/gemini-3.1-pro) still documents USD 2/12 input/output through 200K tokens and 4/18 above; the encoded tier already matches. The [Gemini 3 Pro page](https://replicate.com/google/gemini-3-pro) yielded a README but no independently retrievable current tariff, and direct retrieval returned HTTP 403. Its retained historical pricing is not claimed revalidated. New Gemini transports require model-specific prompt/thinking and receipt qualification; no creator-host rate is copied. |
| `deepl` | [Usage and billing](https://support.deepl.com/hc/en-us/articles/360020685720-Usage-count-and-billing-in-DeepL-API): translation is charged in source characters and plans differ. Retain the explicitly historical regional default and operator overrides; no current regional amount was established. New speech products have separate meters. |
| `ollama` | [Installed models API](https://docs.ollama.com/api/tags): actual tags and quantization belong to each configured installation. Preserve curated compatibility defaults; public model-library membership does not prove local availability or a universal hosted tariff. |
| `copilot` | [Supported AI models](https://docs.github.com/en/copilot/reference/ai-models/supported-models): new display labels include GPT-6.2 and Gemini 4 Neon, but client, plan and credit rules do not establish this adaptor's public inference IDs or per-token USD rates. Preserve channel-configured model behavior. |
| `coze` | [Premium page](https://www.coze.com/premium) returned no readable tariff. The existing adaptor uses configured bot IDs through `/open_api/v2/chat`. Preserve its labeled compatibility defaults; subscription credits are not converted into token prices. |
| `palm` | [Former PaLM documentation](https://ai.google.dev/palm_docs) returned HTTP 404. Retain the clearly historical `PaLM-2` alias; current Gemini models belong to the separate Gemini adaptors. |
| `muapi` | [Model catalog](https://muapi.ai/docs/models) and [pricing](https://muapi.ai/docs/pricing) remain dynamic and model-specific. Keep the existing video quote/receipt settlement and representative discovery IDs; the public text catalog is a separate API surface. |
| `proxy` | Local implementation is a protocol forwarder for channel-configured upstreams. It has no independent provider catalog or universal tariff to refresh. |
| `openai_compatible` | Local implementation contains shared request/response conversion utilities. Provider-owned catalogs remain the sources of IDs, capabilities and rates. |

## Validation boundary

Coordinated validation commands and results are recorded in
[the refresh report](../adaptor-model-refresh-20261122.md). Research used public
read-only sources. No provider inference, production deployment or credential
change is part of this refresh.
