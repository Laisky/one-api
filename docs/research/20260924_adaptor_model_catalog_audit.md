# Adaptor model-catalog audit

## Execution state

- Scope: every registered adaptor and then legacy/provider helper directories.
- Baseline: `4b455cd03126c3358bde5fda19262d97b31e3300` on `main`.
- Working PR: [#425](https://github.com/Laisky/one-api/pull/425), branch `feat/adaptor-catalog-audit-20260924`.
- Research date: 2026-09-24 UTC (2026-09-23 in America/Toronto).
- **Status: partial delivery; NOT a completed all-provider audit. Keep the PR draft.**
- Implemented: five providers, seven new metadata entries, four additional Azure discovery IDs, and 15 Mistral cached-input rates. Eight new top-level regression tests cover metadata and the existing request-conversion paths.
- External inference spend: no paid provider requests were made. No live account eligibility or production deployment was tested.
- Preserve independent work: Groq [#424](https://github.com/Laisky/one-api/pull/424) and MuAPI [#423](https://github.com/Laisky/one-api/pull/423) are not included or modified.

This is an execution ledger, not a claim that untouched entries are current. A partial review does not certify every family, price, capability, or retirement date. Do not mark a provider complete from its root constants file when family files or runtime overrides supply its actual catalog.

## Rules for remaining updates

1. Use official provider model IDs, endpoint documentation, prices, deprecations, and model cards. Keep a dated source for each changed field. A model owner's price is not automatically a reseller's price.
2. Include officially supported preview and enterprise models without an account-permission filter when the relay can actually route them. Never silently rewrite old model IDs or remove historic prices as a side effect of discovery maintenance.
3. Separate USD/CNY, per-token/character/audio-second/image/call rates, cached input, cache writes, promotional periods, and long-context tiers. Unknown pricing is not a zero/free quote.
4. Advertise only capabilities the adaptor can forward. A missing native protocol, tool payload, image conversion, or response decoder needs behavior tests and an implementation change, not just a new name in a map.
5. Use public adaptor discovery/pricing and serialized request tests. Check both fixed IDs and aliases. Preserve operator overrides and avoid turning a published tier limit into an account-specific request denylist.

## Implemented changes and evidence

### Cerebras

Sources: [live public models API](https://api.cerebras.ai/public/v1/models), [model overview](https://inference-docs.cerebras.ai/models/overview), [reasoning](https://inference-docs.cerebras.ai/capabilities/reasoning), [dedicated endpoints](https://inference-docs.cerebras.ai/dedicated/overview).

Added `qwen-3.8-27b` at $0.99 input / $1.49 output per million tokens. Its live public API limits are 65,536 context and 32,768 output; the prose overview separately advertises 128K paid context. The patch records this discrepancy rather than inventing a paid output limit or setting a hard `MaxTokens` cap. Cerebras uses `none/low/medium/high`, default `high`, not Groq's `default` effort value.

Corrected `gpt-oss-120b` maximum output to 40,960 and removed unsupported logprobs/parallel-tool-call advertisements from that model. Dedicated Gemma/GLM compatibility entries remain selectable, with historical prices explicitly labeled as not current enterprise quotes. Tests cover serialized metadata, public getters, reasoning vocabularies, and discovery independence.

### Z.AI

Sources: [USD pricing](https://docs.z.ai/guides/overview/pricing), [GLM-5.3-Flash family](https://docs.z.ai/guides/vlm/glm-5.3-flash), [Chat Completion API](https://docs.z.ai/api-reference/llm/chat-completion), [structured-output guide](https://docs.z.ai/guides/capabilities/struct-output).

Added separately priced `glm-5.3-flashx`: $0.37 input / $0.075 cached input / $1.25 output per million tokens; 1M context, 128K output, text/image/video/file input. It does not inherit BigModel CNY rates or Flash's expired launch promotion. Efforts are `low/high/max`, default `max`. JSON mode is advertised, not an unverified strict `json_schema` guarantee. Tests cover prices, model identity, image preservation, all three efforts, and the native v4 endpoint. Native BigModel FlashX pricing remains unresolved.

### Mistral

Sources: [pricing](https://docs.mistral.ai/inference/pricing), [GLM-5.2](https://docs.mistral.ai/models/zai-glm-5-2), [GLM-5.3](https://docs.mistral.ai/models/zai-glm-5-3), [Leanstral 1.5](https://docs.mistral.ai/models/leanstral-1-5).

Added `zai-glm-5-2` and `zai-glm-5-3` at Mistral's $1.40 / $0.14 cached / $4.40 rates, and the explicitly free preview `labs-leanstral-1-5`. Added 15 published cached-input prices across Medium, Large, Small, Ministral, Codestral, and Codestral Embed without changing their ordinary input/output tariffs. Tests cover public getters, legacy discovery rebuilding, exact IDs, and tool payload preservation.

Remaining: the existing converter drops `ReasoningEffort`; current [reasoning documentation](https://docs.mistral.ai/studio/conversations/reasoning) needs a separate conversion/response behavior fix. Some old output limits look stale but were not changed without a model-specific published limit. OCR/audio/moderation additions are not certified by this chat-focused patch.

### Cohere

Sources: [North Mini Code 1.0](https://docs.cohere.com/docs/north-mini-code-1.0), [North Small Translate 1.0](https://docs.cohere.com/docs/north-small-translate-1.0).

Added `north-mini-code-1-0` (256K context / 64K output) and `north-small-translate-1-0` (16K / 16K). Both cards explicitly describe free API access up to rate limits, including production keys. Dedicated Model Vault instance contracts are not converted into invented token rates. Basic text chat is advertised; native North tool/reasoning features are not advertised because the existing Cohere converter does not forward them. Tests verify the v1 chat payload, exact IDs, limits, and pricing. Legacy estimated Command/rerank prices still require independent re-audit.

### Azure

Source: [Microsoft Foundry Claude models](https://learn.microsoft.com/azure/foundry/foundry-models/concepts/claude-models).

Added discovery for `claude-opus-5-5`, `claude-opus-5`, `claude-fable-5-1`, and `claude-mythos-5-1`. Their metadata already exists in the inherited Anthropic pricing table. This change preserves existing per-family pricing dispatch; it does NOT establish a new claim that every Azure regional/contract price equals first-party Anthropic pricing. Tests verify native Messages routing even after a custom deployment-name mapping. Remaining Azure partner models and regional prices are not certified.

## Provider-by-provider ledger

The registry in `relay/adaptor.go` has 34 adaptors. `Updated` means the scoped changes above are implemented, not that all historic entries are newly certified. `Pending` means the provider was inventoried but not substantively reviewed in this pass.

| Adaptor | Status | Evidence checked / next required work |
| --- | --- | --- |
| aiproxy | Partial: delegation | Read adaptor pricing and library request path. Verify the separate discovery list against delegated OpenAI metadata and operator library semantics. |
| ali | Partial: structure | Root map merges five family maps and uses CNY. Read every family and verify domestic/international rates separately; do not treat the root as the full catalog. |
| anthropic | Partial: current releases | September helpers already contain Opus 5.5, Fable/Mythos 5.1, and permanent Sonnet 5 prices; compared current model overview. Full legacy lifecycle audit remains. |
| aws | Partial: actionable gap | Read Claude pricing and model-ID registries. Opus 5.5 requires coordinated pricing, child model-ID map, regional/profile handling, and native request tests. Do not add an unroutable metadata-only entry. |
| azure | Updated: discovery | Four current Claude IDs added; broader Foundry partner pricing remains open. |
| baidu | Pending | Read catalog families and official Qianfan IDs, regional tariffs, and endpoints. |
| cerebras | Updated | Current public models and dedicated compatibility handled above. |
| cloudflare | Partial: current releases | August helper already adds Qwen 3.8 and dated DeepSeek V4 IDs; compared Workers AI catalog. Audit full base map, model-specific limits, and non-chat native endpoints. |
| cohere | Updated: North | Two North entries added. Legacy prices, tool/reasoning forwarding, and full v2 capability audit remain. |
| copilot | Pending | Check officially available model list and plan multipliers; do not equate a premium request with a per-token USD charge. |
| coze | Pending | Determine bot/deployment identifiers and operator-specific pricing before catalog changes. |
| deepinfra | Partial: actionable gap | Read current text/embedding/audio map and official catalog. GLM-5.3/Flash cards show promotions; inspect all overlays and exact discount lifetime before adding rates. |
| deepl | Partial: alias semantics | Three aliases represent target languages, not LLM IDs. Reviewed constants and language API docs; character pricing and current language/model_type mapping still need verification. |
| deepseek | Partial: pricing windows | Read V4 catalog and official pricing. Resolve temporary Pro-to-Flash routing and time-window overlays before changing billing. |
| fireworks | Partial: structure | Root map merges 12 family maps; official serverless model documentation reviewed. Family-level availability, prices, and dedicated-only compatibility remain. |
| gemini | Partial: delegated catalog | Uses `geminiOpenaiCompatible.ModelRatios`; inspected Gemini entry point and official current models. Audit the shared source, dated aliases, and non-chat endpoints. |
| groq | Separate PR | Refresh is in #424; validate and integrate independently, without duplicating its changes here. |
| jina | Research only | Official model/deployment documentation located. Read adaptor catalog, hosted API IDs, token accounting, and v5/reranker/omni capabilities. |
| mistral | Updated: chat/cache | Three entries and 15 cached rates added. Reasoning, OCR, audio, moderation, and stale output limits remain open. |
| moonshot | Partial: current IDs | Official Kimi catalog matches the four current IDs read in the map. Pricing table rendering and K3 cache-write TTL rates remain unresolved; preserve CNY distinction. |
| nvidia | Research only | Current NIM support matrix located. Distinguish hosted trial endpoints from self-hosted NIM/deployment-specific model IDs and pricing. |
| ollama | Partial: deployment semantics | Read adaptor and official local `/api/tags` contract. Installed models and tags are operator-specific; a global model library is not the installed catalog or a hosted price list. |
| openai | Partial: assembled catalog | Root already applies the September catalog overlay. Verify the overlay and every family before claiming complete currency; official current model index reviewed. |
| openrouter | Partial: large snapshot | Root snapshot and live `/api/v1/models` reviewed. Full snapshot/overlays, per-provider endpoints, long-context overrides, batch variants, and negative pricing sentinels require a reproducible diff. |
| palm | Pending | Check official service retirement and preserve legacy configuration/billing compatibility. |
| proxy | Pending | Inspect inherited discovery versus administrator-defined upstream model mappings. |
| replicate | Pending | Verify versioned model references, deployment routing, and time/image/video-specific prices. |
| tencent | Pending | Read Hunyuan families, native IDs, CNY prices, and modality endpoints. |
| typesafe | Checked: current three IDs | Official models page matches `jev-1.13.0`, `jev-latest`, `jev-preview`; $0.042/M input and free output. No catalog change required for this snapshot. |
| vertexai | Pending | Verify Vertex-specific partner IDs, prices, regions, and inherited Gemini/Claude catalog registration. |
| xai | Partial: recent overlay | September 21 overlay already adds Grok 4.7 and long-context pricing. Full alias and image/audio audit remains. |
| xunfei | Pending | Verify Spark IDs, native protocol differences, and CNY pricing. |
| zai | Updated: FlashX | One independently priced model added; inherited legacy capability declarations still need complete audit. |
| zhipu | Partial: pricing blocked | Read flagship/vision maps and GLM-5.3-FlashX documentation. BigModel price page did not expose readable CNY rows; do not copy Z.AI USD rates or encode unknown as free. |

Legacy provider directories and shared helpers are a second inventory step after the registry. In particular, `geminiOpenaiCompatible` must be audited with Gemini/Vertex. A directory not present in the registry must not be marked as an independently supported channel without checking channel registration.

## Additional sources used for partial reviews

- [Anthropic model overview](https://platform.claude.com/docs/en/models/overview) and [Opus 5.5](https://platform.claude.com/docs/en/models/opus-5-5/overview).
- [AWS Opus 5.5 card](https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-5-5.html) and [Bedrock pricing](https://aws.amazon.com/bedrock/pricing/).
- [OpenAI models](https://developers.openai.com/api/docs/models), [Gemini models](https://ai.google.dev/gemini-api/docs/models), and [xAI models](https://docs.x.ai/developers/models).
- [DeepSeek pricing](https://api-docs.deepseek.com/quick_start/pricing), [Kimi models](https://platform.kimi.com/docs/models), and [Kimi pricing](https://platform.kimi.com/docs/pricing/chat).
- [Cloudflare Workers AI models](https://developers.cloudflare.com/workers-ai/models/), [Fireworks models](https://docs.fireworks.ai/getting-started/models), [DeepInfra models](https://deepinfra.com/models), and [OpenRouter live model API](https://openrouter.ai/api/v1/models).
- [NVIDIA NIM support matrix](https://docs.nvidia.com/nim/large-language-models/latest/supported-architectures.html), [Jina official deployment catalog](https://github.com/jina-ai/jina-on-prem), [Ollama local inventory](https://docs.ollama.com/api/tags), [DeepL supported languages](https://developers.deepl.com/docs/getting-started/supported-languages), and [TypeSafe models](https://docs.typesafe.ai/models).

## Validation and acceptance

Local checks: `gofmt -l` returned no files for all ten changed Go files; Go AST parsing succeeded and found eight top-level tests. These are syntax/format checks, not typechecking or executable regression results. Local Go is 1.23.2 rather than the repository's required 1.27, and GitHub/dependency downloads could not be resolved in this environment. Full local unit/race tests, `go vet`, and frontend build were not run.

CI evidence must remain commit-specific. Initial Cerebras commit `defd23e44b83` passed the package race/coverage shard and repository-wide vet; remaining shards were superseded by later pushes. At `f7e9949cb556`, repository-wide static guardrails including vet, dependency scanning, and the goroutine guard passed; Go shards were still running when inspected. These results do not certify the later Azure or documentation commits. The frontend jobs were skipped by the existing path filter, not manually bypassed.

Before acceptance, require final-head CI, review all new model fixtures against their official sources, and resolve any actual failures without weakening assertions. No full-provider completion or merge readiness is asserted here.

## Next action

First inspect final-head PR CI and fix any evidenced failures in these five provider patches. Then continue with AWS Opus 5.5's native model-ID/pricing integration, BigModel FlashX's independently sourced CNY rate, and the remaining registry rows. Read family files and initialization overlays before creating additions. Update this ledger in place with verified facts and remaining work; do not append a running diary.
