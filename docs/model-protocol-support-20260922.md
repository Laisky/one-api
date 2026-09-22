# Standard model protocol and billing audit

## Status and scope

Baseline: `6c7deddd8d729c4966c7a87496c40b5f74d80a58` (main after xAI video PR #419).
This change delivers the first qualified group of previously missing or incorrectly
metered standard protocols. **It does not complete every native model family in the
catalog.** No paid provider inference was used for verification. A model alias,
listed price, or existing usage example is not proof of a working provider account.

The generated [per-channel inventory](model-protocol-audit-20260922.csv) covers every
runtime catalog entry. A model with the same name on two providers is recorded
separately because its wire protocol and price can differ. The inventory does not
label all missing examples as unsupported. Entries have these meanings:

| Status | Meaning |
| --- | --- |
| `qualified-functional-and-ledger-tests` | This change verifies the stated standard subset using HTTP fixtures and real SQLite quota/task/request-cost records. |
| `existing-example-not-requalified` | The UI already supplies a sample. This audit has not requalified every endpoint, option, price, or upstream account for that model. |
| `native-contract-and-billing-review-required` | A native reference is known but its complete gateway request/response/metering contract still requires work. It may have another working entry point. |
| `no-reviewed-example` | No reviewed example matched. First distinguish a missing UI profile from an actual backend or billing gap; do not infer unavailability. |

## Qualified IDs and standard subsets

| Channel | IDs | Standard gateway entry point and metering |
| --- | --- | --- |
| Cohere | `embed-v4.0`, `embed-english-v3.0`, `embed-english-light-v3.0`, `embed-multilingual-v3.0`, `embed-multilingual-light-v3.0` | `/v1/embeddings` text input, float/base64 output; Cohere v2 `meta.billed_units` rather than raw token counts. |
| Groq | `canopylabs/orpheus-v1-english`, `canopylabs/orpheus-arabic-saudi` | `/v1/audio/speech`, WAV, up to 200 Unicode characters; character billing. |
| Groq | `whisper-large-v3`, `whisper-large-v3-turbo` | `/v1/audio/transcriptions`, one uploaded file; physical duration and the documented 10-second minimum. Turbo translation is rejected. |
| Mistral | `voxtral-mini-tts-2603`, `voxtral-mini-tts-latest`, legacy `voxtral-tts-2603` | `/v1/audio/speech`, non-streaming; standard `voice` maps to `voice_id`, or use `ref_audio`. Decode native base64 audio into binary. Character billing. |
| Mistral | `voxtral-mini-2602`, legacy `voxtral-mini-transcribe-2602` | `/v1/audio/transcriptions`, one uploaded file; physical duration billing. Legacy alias retains its administrator price while the wire uses the canonical model. |
| SiliconFlow | `FunAudioLLM/CosyVoice2-0.5B` | `/v1/audio/speech`; **UTF-8 bytes**, not Unicode character count or estimated text tokens. |
| BigModel / Zhipu | `cogvideox-2`, `cogvideox-3`, `cogvideox-flash`, `viduq1-text`, `viduq1-image`, `viduq1-start-end`, `vidu2-image`, `vidu2-start-end`, `vidu2-reference` | `/v1/videos/generations` (or `/v1/videos`) creates a native job; `/v1/videos/{id}` polls it on the original owner-bound channel. Charge per accepted invocation, including explicitly free tariffs; never charge polling. |
| Z.AI | `cogvideox-3` | Same video gateway subset, with the Z.AI bearer credential format rather than BigModel JWT credentials. |
| SiliconFlow | `black-forest-labs/FLUX.1-schnell`, `black-forest-labs/FLUX-1.1-pro`, legacy `black-forest-labs/FLUX.1.1-pro`, `black-forest-labs/FLUX.2-pro`, `black-forest-labs/FLUX.2-flex`, `Tongyi-MAI/Z-Image-Turbo`, legacy `Bytedance/Z-Image-Turbo` | `/v1/images/generations`; one text-to-image output, native `image_size`, standard `data[].url` response, per-image price. FLUX.2-pro defaults to 512x512 and enforces its documented resolutions. |

These are **31 distinct catalog IDs (including aliases)**, not 31 new underlying
models. Some requests already reached an upstream before this change but used the
wrong payload, response mapping, or billing unit. The UI examples remain at the
bottom of the pricing modal. SiliconFlow image examples require an explicit
provider contract feature, so the same FLUX name on another provider is not
incorrectly assigned the SiliconFlow request.

## Accounting contract

1. Normalize model mapping, duration, input, output count and resolution before
   quota admission. Meter exactly the normalized input that is sent upstream.
2. Prefer direct character, UTF-8 byte, time or per-call tariffs. Preserve explicit
   administrator rate/free overrides and date windows. A metadata-only override
   must not disable the provider tariff. Display these units in the UI.
3. Multiply decimal quantities and divide the price denominator before a single
   ceiling to integer quota. Reject negative, nonfinite or overflowing quantities.
4. Invalid requests and insufficient balances must not invoke the provider. A
   rejected HTTP/error envelope reconciles request cost and refunds any reserve.
5. A valid accepted result remains billable when the caller disconnects. Do not
   automatically replay potentially billable requests after uncertain transport
   or delivery failures. Polls are read-only and quota-free.
6. An unlimited token does not make its user's provider work free; retain the
   existing unlimited-token counter semantics while charging the user ledger.

Per-call video is charged on accepted creation, matching the existing gateway
policy. This change does **not** add a background refund worker for a job that later
fails, or task-store recovery after a persistence outage. Missing task bindings
fail closed. See [xAI video](xai-video.md) for shared polling ownership semantics.

Cohere missing `billed_units` is explicitly recorded as an estimate rather than
silently reported as an exact receipt. Text-only standard embedding inputs are
qualified here; multimodal embedding requests require their own shape and image
metering verification. Existing token-based audio models are not all converted to
input-only direct tariffs: models billed on generated audio need output receipts.

## Remaining native/model groups

The CSV is the exhaustive entry list, not a promise that every entry has been
implemented. The following work remains outside this first qualified group:

| Group | Required next proof before claiming support |
| --- | --- |
| Gemini image models | Existing Chat conversion emits image parts, but requalify output-image versus output-token billing, thought tokens and resolution; separately implement/test the standard Images endpoint. |
| Gemini TTS | Map speech options and native PCM/audio output; meter the provider's text/audio token receipts rather than reusing an input-character estimate. |
| Imagen and Veo on Gemini / Vertex | Verify current provider/channel availability; native prediction/operation polling, owner affinity, output resolution, duration and audio pricing differ. Google has an OpenAI compatibility layer, so a native API is not inherently impossible to standardize. |
| Mistral OCR and other page-based OCR | Implement explicit page receipts, annotation/image surcharges, content mapping and page-based quota admission/refund tests. |
| Alibaba, MiniMax, StepFun and other native speech/image/video families | Pin actual per-family endpoints and input/output media contracts, then add their native billing units and HTTP/ledger fixtures. |
| Replicate / Together / DeepInfra / OpenRouter specialized media | Verify each provider contract and GPU-time/per-megapixel/per-output/token tariffs; a shared model name must not pick another provider's encoding. |
| Remaining named chat families | Many unrecognized names may already work through standard chat. Add a channel-specific profile only after tracing the existing adaptor and testing its request and usage receipt; do not blindly classify by text modality or a model-name regex. |
| Historical or misspelled catalog aliases | Verify the accepted wire ID. Preserve operator model mappings and price overrides; do not silently delete aliases or invent a public release. |

Prioritize the native families most used by the deployment. Each follow-up should
close a named set of CSV entries with function and ledger tests; keep unresolved
entries visible instead of changing all profiles to a generic chat/image sample.

## Repeatable inventory and tests

```sh
go run ./tools/model-protocol-audit /tmp/model-catalog.json
node --experimental-strip-types scripts/model-protocol-audit.mjs \
  /tmp/model-catalog.json docs/model-protocol-audit-20260922.csv
node --experimental-strip-types --test scripts/model-protocol-audit.test.mjs

go test -race -count=1 ./relay/adaptor/cohere ./relay/adaptor/zhipu \
  ./relay/adaptor/zai ./relay/adaptor/openai ./relay/adaptor/siliconflow \
  ./relay/adaptor/groq ./relay/adaptor/mistral ./relay/controller \
  ./relay/pricing ./relay/channeltype ./middleware ./controller ./router
go test -race -count=1 ./model -run 'Test.*(ProtocolAudit|Audio|Video|Pricing|ModelPrice|ModelConfig)'
go vet ./...
(cd web/modern && yarn check:i18n && yarn test --run && yarn build)
```

Install `ffmpeg` for physical audio-duration tests. New tests must not skip missing
metering dependencies or depend on paid credentials. The five embedding regressions
were also run against unchanged main and failed at their intended assertions, not
at compilation. Existing complete PR CI remains required; focused qualification is
not a replacement for repository-wide race/coverage checks.

## Primary protocol and tariff references (reviewed 2026-09-22)

- Cohere Embed v2: https://docs.cohere.com/reference/embed
- Cohere billed versus raw tokens: https://docs.cohere.com/docs/how-does-cohere-pricing-work
- Groq Orpheus: https://console.groq.com/docs/text-to-speech/orpheus
- Groq speech recognition: https://console.groq.com/docs/speech-to-text
- Mistral Voxtral TTS: https://docs.mistral.ai/models/voxtral-tts-26-03
- Mistral speech generation: https://docs.mistral.ai/capabilities/audio/text_to_speech
- Mistral transcription: https://docs.mistral.ai/models/voxtral-mini-transcribe-26-02
- SiliconFlow speech unit: https://docs.siliconflow.cn/docs/userguide/capabilities/text-to-speech
- SiliconFlow images: https://docs.siliconflow.com/en/api-reference/images/images-generations
- SiliconFlow Z-Image model: https://www.siliconflow.com/models/z-image-turbo
- BigModel asynchronous video: https://docs.bigmodel.cn/api-reference/模型-api/视频生成异步
- Google OpenAI compatibility: https://ai.google.dev/gemini-api/docs/openai

## Review follow-up: immediate refund recovery

Rejected audio/video requests now have an internal `quota_refunds` intent with a
server-generated per-attempt ID. The intent is persisted before a credit; its
completion marker and both balance adjustments share one database transaction.
Repeated execution, including after an uncertain commit acknowledgement, cannot
credit the same ID twice. Request retries may reset their Gin markers without
losing the independent pending intent.

Transient enqueue/credit failures receive five bounded attempts. Persisted pending
intents are recovered by the master node at startup and every ten seconds, in
bounded batches, using the existing joined worker lifecycle. Failed intents stay
pending and are deferred rather than blocking later work. A complete database
outage before the intent can be persisted still emits a critical audit record
with its refund ID, owner/token IDs, amount and request ID for reconciliation;
there is no unsafe fallback to an untracked credit. Keep completed IDs for replay
deduplication. Recovery does not change the charge-on-accepted-video policy or
refund jobs that fail asynchronously after acceptance.

The database migration creates `quota_refunds`; roll out a migrating master before
non-migrating replicas, as with other gateway schema changes. Tests cover transient
and persistent financial-write failures, partial-transaction rollback, request
reset/cancellation, duplicate executions and recovery after closing/reopening the
on-disk ledger.
