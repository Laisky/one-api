# Gemini model catalog audit — September 30, 2026

## Scope and sources

This audit records the initial catalog commit of PR #438. Later commits implement the standard `/v1/audio/speech` endpoint; see the [speech implementation guide](gemini/tts/README.md). Model availability still depends on upstream account entitlements. Prices use paid **Standard** Gemini Developer API rates in USD per million tokens. Existing channel overrides remain administrator-owned.

Primary sources reviewed on September 30, 2026:

- [Gemini model catalog](https://ai.google.dev/gemini-api/docs/models).
- [Release notes](https://ai.google.dev/gemini-api/docs/changelog), especially September 22 (TTS GA), September 18 (2.5 access), and September 15 (Live GA).
- [Gemini 3.8 Flash TTS model card](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-tts).
- [Gemini 3.8 Flash-Lite TTS model card](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-lite-tts).
- [Gemini Developer API pricing](https://ai.google.dev/gemini-api/docs/pricing), Standard TTS and Robotics ER 2 sections.
- [Deprecations](https://ai.google.dev/gemini-api/docs/deprecations).
- [Google Cloud Agent Platform pricing](https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing), separately reviewed for backend differences.

The baseline already contains Gemini 3.8 Flash, both Gemini 3.8 Live models, Gemini 3.5 Transcribe, Gemini Omni 1.1 Flash, and Gemini Robotics ER 2. No speculative model IDs or unannounced prices are added.

## Added models

| Model ID | Input/output | Input limit | Output limit | Status |
| --- | --- | ---: | ---: | --- |
| `gemini-3.8-flash-tts` | Text → audio | 8,192 | 16,384 | Developer API GA, September 22, 2026 |
| `gemini-3.8-flash-lite-tts` | Text → audio | 8,192 | 16,384 | Developer API GA, September 22, 2026 |

Both models support audio generation and caching upstream. Neither advertises function calling, structured outputs, thinking, search grounding, or the Live API. Metadata is built independently instead of inheriting the similarly named Flash chat model. The native REST system-instruction allowlist is unchanged.

## Corrected paid Standard rates

Values before and after the arrow apply through December 31, 2026 and from January 1, 2027, respectively.

| Model | Input | Output | Cached input |
| --- | ---: | ---: | ---: |
| Gemini 3.8 Flash TTS | $0.50 → $1.00 | $9.00 → $18.00 audio | $0.125 → $0.25 |
| Gemini 3.8 Flash-Lite TTS | $0.50 → $1.00 | $6.00 → $12.00 audio | $0.125 → $0.25 |
| Gemini Robotics ER 2 Preview | $1.00 → $2.00 | $5.00 → $10.00 text | $0.10 → $0.20 |
| Gemini Robotics ER 2 Streaming Preview | $1.00 → $2.00 | $5.00 → $10.00 text | Not advertised |

The baseline Robotics entries used the future $2/$10 rates immediately and lacked a transition. The correction applies the published current rates and a full-day UTC `DateFrom: 2027-01-01` overlay, consistent with the existing Flash transition convention. Google publishes dates rather than a precise timezone; UTC is this repository's explicit boundary convention.

TTS audio generation consumes 25 output tokens per second. Ten seconds therefore costs $0.00225 for Flash or $0.0015 for Flash-Lite at current Standard output rates, excluding input. Tests reconstruct absolute audio prices using `Ratio * Audio.PromptRatio * Audio.CompletionRatio`, as well as the generic completion fallback. Both calculations must remain correct across the transition. A unit audio prompt multiplier is a billing anchor, not a claim that TTS accepts audio input.

Cache storage, free allowances, regional uplifts, negotiated discounts, and Batch/Flex/Priority consumption options are not folded into ordinary input or output prices. The separate time-based cache-storage charge is not represented by `CachedInputRatio`.

## Lifecycle and compatibility

The September 18 release note limits Gemini 2.5 access to users with prior active usage. It explicitly says the models are **not deprecated** and recommends Gemini 3.8 Flash or Gemini 3.5 Flash-Lite for new projects. The three stable chat entries now explain this distinction without removing their IDs or changing their rates.

The old 3.1 and 2.5 TTS previews retain their original IDs and prices. No shutdown date is announced for those TTS entries in the reviewed deprecation table; migration recommendations are not treated as shutdown announcements. The Omni preview description now labels September 30 as a published **earliest** shutdown date rather than claiming a confirmed shutdown event.

### TTS protocol boundary

The initial catalog commit did not implement Gemini 3.8 TTS transport. The subsequent implementation adds `/v1/audio/speech` over native GenerateContent on Gemini, Gemini OpenAI-compatible, and Vertex channels; see [gemini/tts/README.md](gemini/tts/README.md). Chat, Responses, and Claude conversion paths remain unchanged. Live-provider validation has not been performed. These protocol constraints still apply:

- Speaker/style instructions belong in per-part `speech_metadata`; ordinary input text is treated as a verbatim transcript.
- Multi-speaker turns must identify the speaker in structured metadata.
- Unary responses default to WAV, unlike the headerless PCM defaults of older TTS models. Do not prepend another WAV header.
- Model metadata must not silently redirect an old TTS ID to a new protocol.

### Vertex AI boundary

Vertex AI already consumes the shared Gemini catalog as configuration suggestions, not an entitlement allowlist. The initial catalog change did not alter endpoint selection, regional availability, authentication, or its existing exclusion of Live-only Developer API defaults. The later speech bridge reuses Vertex routing and ADC but requires explicit channel input/output tariffs before dispatch. Google Cloud separately lists the new TTS models as Preview and describes promotional pricing as credits on net spend. Its published effective TTS input/output amounts match the table above, but that does not establish identical caching support, billing-credit eligibility, or account entitlement. Vertex operators must verify backend-specific channel overrides; Developer API lifecycle wording is explicitly scoped to that API.

### Review follow-up

The speech bridge now retains valid `MAX_TOKENS` audio as a terminal truncated result, including buffered formats and trailing usage metadata. SSE completion reports `truncated`; buffered responses include `X-Gemini-Finish-Reason: MAX_TOKENS`. Empty, refused, over-budget, or malformed responses remain errors.

Gateway encoder failures before delivery receive a full customer credit, recorded as `encoding_failed_refund=true` alongside the retained upstream usage. The accepted-work replay guard remains set. Caller cancellation and downstream write failures are not encoder credits. Both pre-reserved and trusted/no-hold requests use the same settlement ledger.

The CI failure in `TestGeminiSpeechHTTP/override` was caused by selecting explicit audio pricing from a ratio-only configuration that deliberately removes media metadata. Speech now resolves the full effective configuration, including time-window audio overrides; an empty audio block still falls back to scalar pricing. Regression tests keep the original 40-quota expectation unchanged and cover exact reservations as well as final charges.

## Regression coverage and validation

Added regression tests cover exact IDs and derived lists, limits/modalities, absence of unsupported chat/reasoning capabilities, independent configuration allocations, legacy price preservation, and shared native-adaptor initialization. Pricing tests call the production `pricing.ApplyTimeWindow` resolver for all four models on the audit date, at the final promotional nanosecond, at the first standard instant, and at the same instant expressed in UTC−5. Existing Robotics expectations are updated to the published current rates.

Initial catalog authoring checks: Go formatting/syntax parsing and exact Git blob SHA verification of both edited baseline files before applying minimal changes. Full project tests were not executable in the authoring container: it has Go 1.23.2, no full checkout, and GitHub DNS resolution is unavailable. The project targets Go 1.27. No paid upstream calls or live model entitlement checks were performed.

Required acceptance commands in a complete checkout with the repository toolchain:

```sh
go test -race ./relay/adaptor/geminiOpenaiCompatible ./relay/adaptor/gemini ./relay/adaptor/vertexai ./relay/pricing
go vet ./...
go test -race ./...
make build-frontend-modern
```

See the PR checks for actual CI results. A test added to the repository is not, by itself, a claim that it has passed.
