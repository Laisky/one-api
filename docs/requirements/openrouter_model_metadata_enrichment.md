# Task: Populate OpenRouter Model Metadata Across All Adaptors

## Background

This project (`one-api`) was recently extended to act as an upstream **provider** for OpenRouter. The integration exposes `GET /openrouter/v1/models` returning the OpenRouter listing schema. Implementation details: see [docs/manuals/openrouter_provider.md](../manuals/openrouter_provider.md).

To support that listing, [relay/adaptor/interface.go](../../relay/adaptor/interface.go) `ModelConfig` was extended with nine optional metadata fields:

| Field | Type | Purpose |
|---|---|---|
| `ContextLength` | `int32` | Total token context (input + output) |
| `MaxOutputTokens` | `int32` | Per-response output cap |
| `InputModalities` | `[]string` | Subset of `text`, `image`, `file` |
| `OutputModalities` | `[]string` | Subset of `text`, `image`, `file` |
| `SupportedFeatures` | `[]string` | Subset of `tools`, `json_mode`, `structured_outputs`, `logprobs`, `web_search`, `reasoning` |
| `SupportedSamplingParameters` | `[]string` | Subset of `temperature`, `top_p`, `top_k`, `min_p`, `top_a`, `frequency_penalty`, `presence_penalty`, `repetition_penalty`, `stop`, `seed`, `max_tokens`, `logit_bias` |
| `Quantization` | `string` | One of `int4`, `int8`, `fp4`, `fp6`, `fp8`, `fp16`, `bf16`, `fp32` |
| `HuggingFaceID` | `string` | HF model id when applicable, empty otherwise |
| `Description` | `string` | Short human-readable description (optional) |

All fields are optional. The mapping layer at [relay/openrouterprovider/mapping.go](../../relay/openrouterprovider/mapping.go) applies fallbacks (`fp16`, `8192`, `4096`, `["text"]`, etc.) when a field is unset. Those fallbacks are **deliberately conservative placeholders** — they keep the listing valid but report incorrect metadata for most models. OpenRouter (and end users) need accurate values for routing, billing display, and capability filtering.

## Goal

Fill in accurate metadata for every model defined in every adaptor's `ModelRatios` map, so `GET /openrouter/v1/models` advertises real context lengths, real modalities, real feature support, and real quantization where known. The pricing fields (`Ratio`, `CompletionRatio`, `CachedInputRatio`, etc.) are already populated and **must not be touched** by this work.

## Scope

The 34 adaptors with `constants.go` ModelRatios maps are:

```
ai360, aiproxy, ali, alibailian, anthropic, baichuan, baidu, baiduv2,
coze, deepseek, doubao, fireworks, gemini, geminiOpenaiCompatible, groq,
lingyiwanwu, minimax, mistral, moonshot, novita, ollama, openai,
openrouter, palm, replicate, siliconflow, stepfun, tencent, togetherai,
xai, xunfei, xunfeiv2, zhipu, anthropic
```

(plus `aws`, `vertexai`, `cohere`, `cloudflare`, `copilot`, `deepl` whose model lists may be defined elsewhere — locate and treat the same way).

## Approach

### Phase 1 — Per-adaptor research

For each adaptor, for each model in its `ModelRatios` map, obtain the following from the **vendor's official documentation** (preferred) or the **HuggingFace model card** (acceptable fallback):

1. **Context length** — total token window. Vendor pricing/specs page is canonical.
2. **Max output tokens** — vendor's documented per-response cap. Use the smaller of the documented values when sources disagree.
3. **Input modalities** — does the model accept image input? File/PDF input? Audio? (Audio/video are *not* OpenRouter-supported input modalities; do not list them in `InputModalities` even if the model supports them — but they may be reflected in pricing sub-configs.)
4. **Output modalities** — usually `["text"]` for chat models. Mark `["image"]` for image-output models, `["text", "image"]` for hybrid (e.g., gpt-image-1).
5. **Supported features** — tools/function-calling, JSON mode, structured outputs, logprobs, web search, reasoning (visible chain-of-thought). Each is a binary per-model fact documented in vendor specs.
6. **Supported sampling parameters** — most OpenAI-compatible models accept the standard set. Reasoning models (o1, o3, gpt-5, deepseek-r1) often **reject** `temperature`/`top_p`/`presence_penalty`. Document the exceptions.
7. **Quantization** — open-weight hosted models often advertise this (e.g., Together.ai serves `bf16` or `fp8` variants). Closed-weight (OpenAI, Anthropic, Google) is unspecified; leave empty.
8. **HuggingFace ID** — open-weight models usually have one (e.g., `meta-llama/Llama-3.3-70B-Instruct`). Closed-weight models have none; leave empty.
9. **Description** — short one-line summary. Optional but nice-to-have.

### Phase 2 — Code edits

Each adaptor's `ModelRatios` is currently constructed via small per-model helper functions or struct literals. Pattern to add the new fields varies but is straightforward — extend the existing literal:

```go
// Example (adaptor/anthropic/constants.go)
"claude-sonnet-4-6": {
    Ratio:           3 * ratio.MilliTokensUsd,
    CompletionRatio: 5,
    CachedInputRatio: 0.3 * ratio.MilliTokensUsd,
    // NEW fields:
    ContextLength:               200_000,
    MaxOutputTokens:             64_000,
    InputModalities:             []string{"text", "image"},
    OutputModalities:            []string{"text"},
    SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
    SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "stop", "seed", "max_tokens"},
    HuggingFaceID:               "", // closed weights
    Description:                 "Anthropic Claude Sonnet 4.6 — balanced reasoning and tool use.",
},
```

If an adaptor uses a builder helper (e.g., `openRouterModelConfig(input, output, ...)`), extend the helper to accept the new fields **with sensible defaults** so existing call sites don't all need editing simultaneously. Keep helper signatures backward-compatible.

### Phase 3 — Verification

For each adaptor edited:
- `go vet ./...` clean.
- `go test -race ./relay/adaptor/<adaptor>/...` passes.
- Spot-check via `go test -race ./relay/openrouterprovider/...` and the controller test in [controller/openrouter_provider_test.go](../../controller/openrouter_provider_test.go) — they should still pass without modification.

For the system as a whole, run the full suite at the end:
```bash
go vet ./...
go test -race ./...
```

## Sources of truth

In priority order:

1. **Vendor's pricing or model docs page** (e.g., `platform.openai.com/docs/models`, `docs.anthropic.com/en/docs/about-claude/models`, `ai.google.dev/gemini-api/docs/models`).
2. **Vendor's release blog post** for the specific model version.
3. **HuggingFace model card** for open-weight models.
4. **OpenRouter's own listing** at `openrouter.ai/api/v1/models` (every entry already has these fields; useful as a sanity cross-check, not as primary source).

Avoid third-party blog summaries — they go stale fast.

## Quality bar

- **Accuracy over completeness**: leave a field empty if you cannot find a definitive source. Empty falls back to the safe default in [relay/openrouterprovider/mapping.go](../../relay/openrouterprovider/mapping.go); a wrong value misleads OpenRouter routing.
- **Cite sources in commits**: each PR's commit message should list the vendor doc URL for the values added (especially `ContextLength` and `MaxOutputTokens`, which are most likely to drift across model revisions).
- **One adaptor per PR** is preferred — keeps review tractable. Bundle small/related adaptors (e.g., the Chinese-cloud cluster: ali, alibailian, baidu, baiduv2, tencent, zhipu) when natural.

## Hard constraints (project policy)

- **English only** — code, comments, commit messages, PR descriptions.
- **Error wrapping** — use `github.com/Laisky/errors/v2` (`Wrap`/`Wrapf`/`WithStack`); never return bare errors.
- **Comments** — every function/type comment must start with the function or type name.
- **File length** — Go files preferably ≤ 600 lines. If a `constants.go` would exceed this after enrichment, split by model family (e.g., `constants_gpt4.go`, `constants_o_series.go`).
- **Logger usage** — request-scoped logger via `gmw.GetLogger(c)` once per function (not relevant to constants, but applies to any controller-level changes).
- **Do not modify pricing fields** (`Ratio`, `CompletionRatio`, `CachedInputRatio`, `CacheWrite5mRatio`, `CacheWrite1hRatio`, `Tiers`, `Image`, `Audio`, `Video`, `Embedding`). They are out of scope and have their own audit cycle.
- **Do not change adaptor interfaces or method signatures** — this is purely additive data.
- **Do not introduce new dependencies** to `go.mod`.
- **Tests** — when adding/changing a field, ensure existing tests still pass. Adding a small assertion that a sampled model has the expected `ContextLength` is welcome but not required.

## Risks and pitfalls

1. **Reasoning models reject standard sampling params** — OpenAI o-series, o1, gpt-5, deepseek-r1, anthropic's `extended-thinking`. Their `SupportedSamplingParameters` lists are short (often only `seed`, `max_tokens`).
2. **Vision models are not always input-image** — some vendors call models "multimodal" but only support image *output* (e.g., DALL-E). Read carefully.
3. **Quantization is often unknown for closed-weight models** — leave empty, do not guess.
4. **Aliases and snapshots** — vendors offer multiple ids for the same logical model (e.g., `gpt-4o`, `gpt-4o-2024-08-06`). Treat each id independently; values may differ when the vendor changed limits between snapshots.
5. **Tiered context windows** — some models (e.g., Gemini 1.5 Pro 2M) advertise a larger context only on specific endpoints or tiers. Use the value advertised on the endpoint this project actually calls (typically the standard chat completions endpoint).
6. **Tool-call support depends on endpoint, not just model** — confirm the model supports tools *via the chat completions endpoint* (some models only support tools through the Assistants/Responses API). If unsupported on chat, omit `tools` from `SupportedFeatures`.
7. **Re-running OpenRouter scrape is cheap** — operators can re-scrape after each PR lands, so iterative landing is fine.

## Definition of done

- All adaptors listed in `Scope` have at minimum `ContextLength`, `MaxOutputTokens`, and `InputModalities` populated for every model in their `ModelRatios`.
- Reasoning models have correct `SupportedSamplingParameters` (the restricted set).
- Vision-input models have `"image"` in `InputModalities`.
- Tool-call-capable models have `"tools"` in `SupportedFeatures`.
- Open-weight models have non-empty `HuggingFaceID` and `Quantization`.
- `go vet ./...` and `go test -race ./...` clean.
- A short summary report (one paragraph per adaptor) lists how many models were enriched and which fields were left empty due to missing sources.

## Pointers

- Field definitions: [relay/adaptor/interface.go:21](../../relay/adaptor/interface.go#L21).
- Mapping logic + defaults: [relay/openrouterprovider/mapping.go](../../relay/openrouterprovider/mapping.go).
- Controller and route: [controller/openrouter_provider.go](../../controller/openrouter_provider.go), [router/relay.go](../../router/relay.go).
- Pricing helper conventions per adaptor: see e.g. [relay/adaptor/openrouter/constants.go](../../relay/adaptor/openrouter/constants.go) for a builder pattern, or [relay/adaptor/anthropic/constants.go](../../relay/adaptor/anthropic/constants.go) for direct struct literals.
- OpenRouter spec used for validation: <https://openrouter.ai/docs/guides/get-started/for-providers>.

## Suggested execution order

1. **High-volume / high-value vendors first**: openai, anthropic, gemini, openrouter, deepseek, xai, mistral, groq, fireworks, togetherai.
2. **Chinese cloud cluster**: ali, alibailian, baidu, baiduv2, tencent, zhipu, doubao, moonshot, lingyiwanwu, baichuan, stepfun, minimax, xunfei, xunfeiv2, ai360.
3. **Long tail**: replicate, novita, siliconflow, ollama, palm, copilot, coze, cohere, cloudflare, deepl, aiproxy, aws, vertexai, geminiOpenaiCompatible.

Ship one adaptor per PR; merge as each is reviewed. Re-scrape OpenRouter after each merge — improvements compound.
