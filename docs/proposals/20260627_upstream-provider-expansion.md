# Change Manual: Upstream API Provider Expansion

- **Status:** Proposal
- **Date:** 2026-06-27
- **Scope:** Add a prioritized set of mainstream LLM API providers currently missing from one-api
- **Owner:** Relay / Adaptors
- **Related memory:** `reference_new_openai_compatible_channel`, `reference_provider_quirks_scope_by_upstream`, `feedback_billing_kind_abstraction`, `feedback_readme_new_models`, `feedback_error_wrapping`

---

## 1. Background

### 1.1 Motivation

one-api currently exposes **56 channel types** (`relay/channeltype/define.go`). An industry gap analysis (6-segment survey + adversarial verification, 66 unique candidates) found a number of **mainstream, currently-live** upstream providers that are not yet supported and that users can only reach today via the generic `OpenAICompatible` fallback (which does **not** consult a provider-specific `ModelRatios`, so billing is approximate) or via OpenRouter (indirect, extra hop and markup).

The verification pass also corrected one baseline assumption: the AWS Bedrock adaptor already covers **multi-vendor** Bedrock (`claude`, `cohere`, `mistral`, `llama3`, `deepseek`, `qwen`, `openai`, `writer`, Amazon Nova in `relay/adaptor/aws/ratios.go`), so Bedrock Nova/Llama/Mistral are **not** in scope.

### 1.2 Selected providers

Selection criteria: live as of 2026, genuinely not already supported, public per-token pricing (required for correct billing), recognizable brand or unique model access, and reasonable integration effort.

| Phase | Provider | Channel name | Base URL | API style | Unique value | Effort |
|---|---|---|---|---|---|---|
| 1 | **Hugging Face Inference Providers** | `HuggingFace` | `https://router.huggingface.co/v1` | OpenAI-compatible (chat only) | One key → 200+ open-weight models across 20+ partner backends | Low |
| 1 | **Perplexity (Sonar)** | `Perplexity` | `https://api.perplexity.ai` | OpenAI-compatible | Search-grounded / citation models + deep-research | Low |
| 1 | **DeepInfra** | `DeepInfra` | `https://api.deepinfra.com/v1/openai` | OpenAI-compatible | High-volume cheap inference, 190+ models, cached-input pricing | Low |
| 2 | **SambaNova Cloud** | `SambaNova` | `https://api.sambanova.ai/v1` | OpenAI-compatible | Recognizable AI-silicon brand; fast inference | Low |
| 2 | **Voyage AI** | `VoyageAI` | `https://api.voyageai.com/v1` | OpenAI-ish embeddings + Cohere-style rerank | Best-in-class embeddings/rerank (MongoDB) | Medium |

**Out of scope (future proposals):** image/video/audio providers (FLUX, fal.ai, Runway, ElevenLabs — non-OpenAI protocols, per-call/per-second billing), the Chinese regional set (SenseNova, ModelScope, PPIO, Gitee AI, Infini-AI), and enterprise clouds requiring custom auth/signing (Databricks, watsonx, OCI, Snowflake Cortex). Gateways (Vercel AI Gateway, Requesty, Portkey, etc.) are redundant with OpenRouter and excluded.

### 1.3 Design rule: dedicated-apitype, not fallthrough

Each provider **must** be added as a **dedicated apitype** (the Fireworks/XAI/DeepSeek/Groq/Cerebras pattern), not as a fallthrough-to-OpenAI channel (the Novita/SiliconFlow/Together pattern). Only the dedicated-apitype path guarantees the provider's own `ModelRatios` is consulted at runtime billing (Layer 2 of `relay/pricing/global.go::GetModelRatioWithThreeLayers`, an existence check where `Ratio:0` means genuinely free). Fallthrough channels resolve to `openai.Adaptor` and never see provider pricing.

### 1.4 Billing quirks that drive the design (per provider)

- **Hugging Face:** price is **per-provider, not per-model** — the same model id bills differently depending on routing policy (`:fastest` default vs `:cheapest` / `:provider` suffix). There is no single static per-model price table. **Mitigation:** register model ids pinned to a specific provider suffix, or mark variable models as approximate. The OpenAI-compatible `/v1` path is **chat-completions only** (embeddings/image use separate HF task endpoints — out of scope). Org billing uses the `X-HF-Bill-To` header.
- **Perplexity:** in addition to token cost, every request carries a **per-request search surcharge** that varies with `search_context_size` (low/medium/high). Pure per-token billing **under-charges**. **Mitigation:** add a per-call surcharge component, reusing the existing `perplexity_native_search_{low,medium,high}` and `perplexity_pro_native_search_*` `UsdPerCall` constants already present in `relay/adaptor/openrouter/constants.go`.
- **DeepInfra:** supports **cached-input pricing** (input vs cached-input rate). **Mitigation:** map to one-api's cache-tier billing (see existing cache-tier handling, e.g. `reference_cny_host_cache_pricing`).
- **SambaNova:** **speed-tier variants** share a base name but differ ~20× in price (e.g. `DeepSeek-V3.1-cb` $0.15/$0.75 vs `DeepSeek-V3.1` $3.00/$4.50). **Mitigation:** register each tier as a **distinct model id** in `ModelRatios` with its own rate — do not collapse them.
- **Voyage AI:** embeddings/rerank only (no chat). **Mitigation:** bill via the embedding/PerCall billing kind (see `feedback_billing_kind_abstraction` — name the kind after billing semantics, not model type); use `siliconflow` (chat+embeddings) and `cohere` (rerank) adaptors as templates.

### 1.5 Routing-safety constraint

Provider quirks (apitype, pricing, base URL) must be gated on **channel type or base URL**, never on model name. The `response_fallback.go` deepseek-prefix bug (a model-name → apitype override that mis-routed third-party DeepSeek hosts onto the DeepSeek adaptor and its pricing) is the cautionary precedent. Because DeepInfra/HuggingFace/SambaNova all host `deepseek-ai/*` open weights, confirm `shouldRouteResponseFallbackThroughDeepSeek` / `isDeepSeekUpstream` still scopes correctly for the new channels (see `reference_provider_quirks_scope_by_upstream`).

---

## 2. Change List

### 2.1 Shared wiring recipe (per provider)

Each new provider touches the **same 15 sites** as the just-merged Cerebras adaptor (`git show e27b74b7 --stat`). Use Cerebras as the copy template for a dedicated OpenAI-compatible chat channel; use `siliconflow` for chat+embeddings and `cohere` for rerank.

**Backend — apitype registration**
1. `relay/apitype/define.go` — add a const **before `Dummy`** (do not renumber existing).
2. `relay/apitype/helper.go` — add the `String()` case.

**Backend — channel type registration**
3. `relay/channeltype/define.go` — add a const **before `Dummy`**.
4. `relay/channeltype/url.go` — add the base-URL entry (index must match the const). `init()` asserts `len(ChannelBaseURLConfigs) == Dummy`.
5. `relay/channeltype/helper.go` — add the `ToAPIType()` case (→ new apitype) **and** the `IdToName()` case.
6. `relay/channeltype/endpoints.go` — add the `DefaultEndpointsForChannelType` case.

**Backend — adaptor**
7. `relay/adaptor.go` — add the import and the `GetAdaptor` case (`relay/adaptor_test.go::TestGetAdaptor` asserts every apitype `< Dummy` returns non-nil).
8. `relay/adaptor/<name>/adaptor.go` — implement the `Adaptor` interface (`GetRequestURL`, `SetupRequestHeader` Bearer auth, `ConvertRequest`, `DoRequest`, `DoResponse`, `GetModelList`, `GetChannelName`, `GetDefaultModelPricing`, `GetModelRatio`, `GetCompletionRatio`, `DefaultToolingConfig`).
9. `relay/adaptor/<name>/constants.go` — `ModelRatios map[string]adaptor.ModelConfig`. Pricing convention: `Ratio = <USD per 1M input> * ratio.MilliTokensUsd`; `CompletionRatio = <USD per 1M output> / <USD per 1M input>`. Set `ContextLength`, `MaxOutputTokens`, modalities, `SupportedFeatures`, `HuggingFaceID`, `Description`. **Only register models with a publicly published per-token price** (omit "coming soon" / unpriced models rather than fabricate a rate).
10. `relay/adaptor/<name>/adaptor_test.go` — table tests for `GetRequestURL` (base-URL/`/v1` handling), header auth, model-list non-empty, and pricing existence.

**Frontend — channel option in all three UIs**
11. `web/air/src/constants/channel.constants.js`
12. `web/berry/src/constants/ChannelConstants.js`
13. `web/modern/src/pages/channels/constants.ts` (and check `web/modern/src/pages/channels/ChannelsPage.tsx` for any hardcoded list/grouping).

**Docs**
14. `README.md` — add to the synopsis provider list, add a "`<Name>` Features" section, and add the TOC entry (see `feedback_readme_new_models`: also update any per-family model list the new models belong to).

**Base-URL `/v1` gotcha:** `versionSuffixRe = /v\d+[a-zA-Z0-9]*$`. If the base URL ends in `/v1`, `GetFullRequestURL` strips the leading `/v1` from the relay path (no doubled `/v1`). Both `.../v1` and the bare host work — DeepInfra (`/v1/openai`) and HuggingFace (`/v1`) are fine.

**Error handling:** never return bare errors — wrap with `github.com/Laisky/errors/v2` (`Wrap`/`Wrapf`/`WithStack`) per `feedback_error_wrapping`.

### 2.2 Per-provider deltas (beyond the shared recipe)

| Provider | Constant names | Models to register (initial) | Extra work |
|---|---|---|---|
| **HuggingFace** | `apitype.HuggingFace`, `channeltype.HuggingFace` | `openai/gpt-oss-120b`, `meta-llama/Llama-3.3-70B-Instruct`, `deepseek-ai/DeepSeek-R1`, `deepseek-ai/DeepSeek-V3-0324`, `Qwen/Qwen3-*`, `zai-org/GLM-*` — chat only | Decide provider-suffix policy; document that prices are per-provider/approximate; optional `X-HF-Bill-To` passthrough. Scope to chat (no embeddings/image). |
| **Perplexity** | `apitype.Perplexity`, `channeltype.Perplexity` | `sonar`, `sonar-pro`, `sonar-reasoning`, `sonar-reasoning-pro`, `sonar-deep-research` | Add per-request **search surcharge** (reuse `perplexity_native_search_*` UsdPerCall). Preserve/handle extra response fields (`citations`, `search_results`) — non-fatal but should not break passthrough. |
| **DeepInfra** | `apitype.DeepInfra`, `channeltype.DeepInfra` | `deepseek-ai/DeepSeek-V3.x`, `meta-llama/Llama-3.x/4.x`, `Qwen/Qwen3-*`, `moonshotai/Kimi-K2`, `openai/gpt-oss-120b` | Map **cached-input** rate to cache-tier billing. |
| **SambaNova** | `apitype.SambaNova`, `channeltype.SambaNova` | Both base and `-cb` tiers as **distinct ids** (e.g. `DeepSeek-V3.1`, `DeepSeek-V3.1-cb`, `Meta-Llama-3.3-70B-Instruct`, `gpt-oss-120b`) | Each tier gets its own `Ratio`/`CompletionRatio`. |
| **VoyageAI** | `apitype.VoyageAI`, `channeltype.VoyageAI` | Embeddings: `voyage-4-large/4/4-lite`, `voyage-3.5/3.5-lite`, `voyage-code-3`; Rerank: `rerank-2.5/2.5-lite` | Embeddings + rerank routing only (no chat). Use embedding/PerCall billing kind; template = `siliconflow` (embeddings) + `cohere` (rerank). |

---

## 3. Test Matrix

Legend: ✅ required · ➖ N/A. "Live smoke" requires a real API key and is run manually (or in a gated CI job), not in unit CI.

| Test dimension | How / where | HuggingFace | Perplexity | DeepInfra | SambaNova | VoyageAI |
|---|---|:--:|:--:|:--:|:--:|:--:|
| **T1 — apitype/channeltype wiring** | `go build ./...`; `init()` assert `len==Dummy` passes | ✅ | ✅ | ✅ | ✅ | ✅ |
| **T2 — `GetAdaptor` non-nil** | `relay/adaptor_test.go::TestGetAdaptor` | ✅ | ✅ | ✅ | ✅ | ✅ |
| **T3 — `GetRequestURL` / base-URL `/v1`** | `adaptor_test.go` table test (no doubled `/v1`; bare host + `/v1` both work) | ✅ | ✅ | ✅ | ✅ | ✅ |
| **T4 — Bearer auth header** | `adaptor_test.go` `SetupRequestHeader` | ✅ | ✅ | ✅ | ✅ | ✅ |
| **T5 — Model list non-empty & matches `ModelRatios`** | `GetModelListFromPricing` test | ✅ | ✅ | ✅ | ✅ | ✅ |
| **T6 — Pricing existence (every model has a `ModelConfig`)** | `relay/pricing` test; Layer-2 lookup hits provider map | ✅ | ✅ | ✅ | ✅ | ✅ |
| **T7 — `/v1/models` listing** | `controller/model.go` apitype loop surfaces models | ✅ | ✅ | ✅ | ✅ | ➖ (embeddings/rerank) |
| **T8 — Chat completion (non-stream)** live smoke | real key, `chat/completions` | ✅ | ✅ | ✅ | ✅ | ➖ |
| **T9 — Chat completion (stream/SSE)** live smoke | real key, `stream:true` | ✅ | ✅ | ✅ | ✅ | ➖ |
| **T10 — Embeddings** live smoke | `embeddings` endpoint | ➖ | ➖ | ✅ | ➖ | ✅ |
| **T11 — Rerank** live smoke | rerank endpoint | ➖ | ➖ | ➖ | ➖ | ✅ |
| **T12 — Token accounting correctness** | usage returned & quota debited == expected for known prompt | ✅ | ✅ | ✅ | ✅ | ✅ |
| **T13 — Provider-specific billing quirk** | see below | ✅ per-provider suffix price | ✅ **per-call search surcharge applied** | ✅ **cached-input tier** | ✅ **`-cb` tier priced distinctly** | ✅ per-token embedding/rerank |
| **T14 — Routing-safety (no model-name hijack)** | confirm `deepseek-ai/*` on this channel keeps this channel's adaptor+pricing, not DeepSeek's | ✅ | ➖ | ✅ | ✅ | ➖ |
| **T15 — Frontend channel option (air/berry/modern)** | manual: channel appears, base URL prefilled, save works | ✅ | ✅ | ✅ | ✅ | ✅ |
| **T16 — README/docs updated** | provider list + Features section + TOC | ✅ | ✅ | ✅ | ✅ | ✅ |
| **T17 — Error wrapping** | grep adaptor for bare `return err`; all wrapped via `errors/v2` | ✅ | ✅ | ✅ | ✅ | ✅ |

**Billing-quirk detail for T13** (the highest-risk tests — bill correctly or not at all):
- **Perplexity:** issue a request with `search_context_size: high`; assert debited quota = token cost **plus** the matching `perplexity_native_search_high` (or `_pro_*`) `UsdPerCall`. A pure-token result is a failure.
- **SambaNova:** request `DeepSeek-V3.1-cb` and `DeepSeek-V3.1` with identical prompts; assert the two debits differ (~20×), proving distinct `ModelRatios` entries.
- **DeepInfra:** a cache-hit request bills the cached-input rate, not the full input rate.
- **HuggingFace:** a pinned provider-suffix model bills the pinned rate; document that unpinned (`:fastest`) is approximate.

---

## 4. Acceptance Criteria

A provider is **Done** when all of the following hold:

1. **Builds & unit tests green:** `go build ./...` and `go test ./relay/...` pass, including `TestGetAdaptor` and the new `relay/adaptor/<name>/adaptor_test.go`. The `channeltype` `init()` length assertion passes.
2. **Dedicated apitype:** the channel resolves through its **own** adaptor (verified by `ToAPIType`), and runtime billing consults the provider's `ModelRatios` (Layer 2), not `openai.Adaptor`. A model priced `Ratio:0` is intentional/free, never an accident of fallthrough.
3. **Pricing accuracy:** every registered model has a `ModelConfig` with `Ratio`/`CompletionRatio` derived from the provider's **published** per-token price (cite the source URL + retrieval date in `constants.go`, as Cerebras does). No fabricated rates; unpriced models are omitted.
4. **Billing quirks honored (T13):** Perplexity per-call search surcharge applied; SambaNova `-cb` tiers priced distinctly; DeepInfra cached-input tier mapped; HuggingFace per-provider pricing documented/pinned. Token accounting (T12) matches expected debit for a known prompt within rounding.
5. **Live smoke passes (T8–T11):** with a real key, non-stream + stream chat works (HuggingFace/Perplexity/DeepInfra/SambaNova); embeddings + rerank work (VoyageAI/DeepInfra). Responses surface usage; extra fields (Perplexity citations) do not break passthrough.
6. **Routing-safety (T14):** hosting `deepseek-ai/*` (or any third-party open weight) on the new channel keeps the channel's own adaptor and pricing — no model-name → apitype hijack.
7. **Frontend (T15):** the channel is selectable in `air`, `berry`, and `modern` UIs with the correct default base URL; creating and testing a channel succeeds.
8. **Docs (T16):** `README.md` updated (synopsis list + Features section + TOC + relevant per-family model list).
9. **Code quality:** all errors wrapped via `github.com/Laisky/errors/v2`; no bare returns. Provider quirks gated on channel type / base URL, never model name.
10. **i18n:** if any new user-visible string keys are introduced, all 5 locales (en/zh/es/fr/ja) are updated (per `feedback_i18n_all_locales`). Channel option labels in the JS/TS constants files are not i18n keys and are exempt.

### Phase gate

- **Phase 1 ships** when HuggingFace, Perplexity, and DeepInfra each meet criteria 1–10.
- **Phase 2 ships** when SambaNova and VoyageAI each meet criteria 1–10.

---

## Appendix A — Reference template (Cerebras, commit `e27b74b7`)

Files changed when Cerebras was added (the canonical 15-site recipe):

```
README.md
relay/adaptor.go
relay/adaptor/cerebras/adaptor.go
relay/adaptor/cerebras/adaptor_test.go
relay/adaptor/cerebras/constants.go
relay/apitype/define.go
relay/apitype/helper.go
relay/channeltype/define.go
relay/channeltype/endpoints.go
relay/channeltype/helper.go
relay/channeltype/url.go
web/air/src/constants/channel.constants.js
web/berry/src/constants/ChannelConstants.js
web/modern/src/pages/channels/ChannelsPage.tsx
web/modern/src/pages/channels/constants.ts
```

## Appendix B — `ModelConfig` pricing convention (from `relay/adaptor/cerebras/constants.go`)

```go
"gpt-oss-120b": {
    Ratio:            0.35 * ratio.MilliTokensUsd, // USD per 1M input tokens
    CompletionRatio:  0.75 / 0.35,                 // (USD/1M output) / (USD/1M input)
    ContextLength:    131072,
    MaxOutputTokens:  40000,
    InputModalities:  textInputs,
    OutputModalities: textOutputs,
    SupportedFeatures: reasoningFeatures,
    HuggingFaceID:    "openai/gpt-oss-120b",
    Description:      "...",
},
```
