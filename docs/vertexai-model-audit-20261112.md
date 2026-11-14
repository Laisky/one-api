# Vertex model compatibility audit — 2026-11-12

## Scope and qualification

The Vertex channel now owns its suggestion catalog instead of advertising the
Gemini Developer API pricing catalog wholesale. Suggestions describe applicable
routes, not project entitlement, regional availability, quota, or live acceptance.
Restricted and preview entries require the corresponding Google access. Existing
model mappings, historical prices and legacy adapter registrations remain available;
removing a suggestion does not delete an existing channel configuration.

This is a documentation and offline transport qualification. No paid Vertex call,
credential creation, deployment or universal live-model qualification occurred.
The older October audit remains historical evidence. Lifecycle dates below are
those documented when this audit was performed, not automatic future route removal.

## Official evidence

Google documentation redirects some older Vertex URLs to Agent Platform.
These canonical references were checked on 2026-11-12:

- [Model versions and lifecycle](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/model-versions)
- [Hydrogen model](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-hydrogen) and [request guide](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/guides/gemini-4-hydrogen)
- [Carbon model](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-carbon)
- [Robotics ER 2](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/robotics-er-2)
- [Veo 3.1](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/veo/3-1-generate)
- [Google tariffs](https://cloud.google.com/model-garden/pricing)
- [Partner tariffs](https://cloud.google.com/model-garden/partner-models/pricing)
- [Open-model deprecations](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/deprecations/open-models)
- [Partner-model deprecations](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/deprecations/partner-models)
- [Partner model overview](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models)
- [Claude on Vertex](https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai)
- [Open MaaS models](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/use-open-models)

## Advertised route and lifecycle matrix

Exact suggestions are maintained in relay/adaptor/vertexai/catalog.go, independent
of pricing-map keys. The following table describes that catalog's qualification.

| Models | Route | Lifecycle, access and limits |
| --- | --- | --- |
| gemini-4-hydrogen | Google v1 generateContent / streamGenerateContent | Current; foreground maximum 65,536 output tokens. Unsupported Hydrogen thinking settings are rejected before transport. |
| gemini-4-argon | Native Gemini | Current. |
| gemini-4-carbon; gemini-4-carbon-cyber-permissive; gemini-4-argon-cyber-permissive; gemini-3.8-flash-cyber | Native Gemini | Restricted; inclusion does not grant enterprise or cyber entitlement. Argon Cyber retains this relay's conservative explicit operator-pricing policy: positive input, output and cache-read prices (implicit caching does not require cache-write prices). This is not a claim that Google has no published tariff; see the November 13 recheck below. Other entries use independently verified standard global tariffs. |
| gemini-3.8-flash; gemini-3.7-flash; gemini-3.6-flash; gemini-3.5-flash; gemini-3.5-flash-lite; gemini-3.1-flash-lite | Native Gemini | Documented served IDs. 3.6 retirement November 19, 2026; 3.7 retirement January 28, 2027. Developer lifecycle descriptions are not authoritative for Vertex. |
| gemini-3.1-pro-preview; gemini-3-flash-preview; gemini-robotics-er-2-preview | Native Gemini | Preview. Robotics requires applicable global endpoint and documented model limits; this does not implement a separate streaming Robotics protocol. |
| gemini-nano-banana-2.1; gemini-3.1-flash-lite-image; gemini-3-pro-image; gemini-3.1-flash-image; gemini-2.5-flash-image | Native Gemini image generation | Documented image IDs. Model-specific resolution, modality and regional restrictions still apply. |
| gemini-3.8-live; gemini-live-2.5-flash-native-audio | Native Vertex Live WebSocket | Explicit operator Vertex realtime pricing required; Developer Live tariffs are never imported. |
| claude-fable-5; claude-fable-5-1; claude-fable-5-2 | Anthropic rawPredict / streamRawPredict | Documented Vertex IDs; pricing qualification remains separate. |
| claude-opus-4-5@20251101; claude-opus-4-6; claude-opus-4-7; claude-opus-4-8; claude-opus-5; claude-opus-5-5 | Anthropic native | Current documented IDs; regional premiums require channel configuration. |
| claude-sonnet-4-5@20250929; claude-sonnet-4-6; claude-sonnet-5; claude-sonnet-5-5 | Anthropic native | [Sonnet 4.5 existing-customer access retires November 30, 2026](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/deprecations/partner-models); no invented fast alias. |
| claude-haiku-4-5@20251001; claude-haiku-5-5 | Anthropic native | Documented IDs; Haiku 5.5 has prompt-length price tiers. |
| veo-3.1-generate-001; veo-3.1-fast-generate-001 | Google predictLongRunning | GA; audio-dependent pricing remains incompletely represented by generic video fields. |
| veo-3.1-lite-generate-001 | Google predictLongRunning | Preview, us-central1, fixed quota / Provisioned Throughput; no assumption of pay-as-you-go access. |
| openai/gpt-oss-120b-maas | Vertex OpenAI-compatible chat/completions | Current; no invented cached-input tariff. |
| deepseek-ai/deepseek-v4.1-flash-maas | Vertex OpenAI-compatible chat/completions | [Preview](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/deepseek/deepseek-v41-flash), released October 14, 2026; minimum availability November 28, 2026. Verify project access. |
| zai-org/glm-5.2-maas | Vertex OpenAI-compatible chat/completions | [Preview](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/zaiorg/glm-52), global; released August 24, 2026, minimum availability November 30, 2026. |
| google/gemma-4-26b-a4b-it-maas | Vertex OpenAI-compatible chat/completions | [Experimental](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/google/gemma-4-26b-a4b-it), global, released April 3, 2026; no explicit minimum availability. Card/guide reasoning guidance conflicts, so reasoning is not advertised. |
| meta/llama-4-maverick-17b-128e-instruct-maas; meta/llama-4-scout-17b-16e-instruct-maas | Vertex OpenAI-compatible chat/completions | GA April 29, 2025; [Scout](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/llama/llama4-scout) and [Maverick](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/llama/llama4-maverick) use us-east5 only. This is separate from Google global routing. |

The current official managed-open inventory lists exactly six models: the new DeepSeek, Gemma, GLM and two Llama IDs above, plus GPT-OSS 120B. All six now have explicit dispatch. Historical MiniMax M2, Kimi K2 Thinking, GLM 4.7 and GLM 5 MaaS models retired October 21, 2026; their published historical prices are not evidence of current service availability. [MaaS inventory](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/use-open-models), [retirement table](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/deprecations/open-models).

## Prices and metering boundaries

Tariffs are USD per million tokens unless otherwise stated. Google's standard
global Hydrogen rates are 1.50/7.50 with cache reads .075; Carbon is 4/20/.20.
Promotional account credits are not unconditional request-price reductions.
Regional, priority, batch, provisioned-throughput and account discounts are separate.
Gemini 3/4 grounding charges individual queries; generic per-call defaults cannot
represent that faithfully. Veo charges vary jointly with audio and resolution;
a resolution-only multiplier is insufficient for every combination. Imagen
upscaling is a separate operation tariff, not ordinary generation.

Partner source confirms existing Qwen, DeepSeek V3 and GPT-OSS tariffs without
making retired models current. DeepSeek V4.1 Flash is .30/1.20/.03; its free period
ended October 16. Fable 5.2 is 8/40/.20 globally, cache writes 10/16.
GLM 5.2 is 1.40/4.40/.14; Gemma 4 is .15/.60/.015.
Llama Scout is .25/.70 and Maverick .35/1.15, without invented cache rates.
Fable and Mythos pricing admission must follow the actual explicit-pricing policy;
availability of an Anthropic first-party rate is insufficient Vertex evidence.

Existing user-defined pricing and aliases are retained. Legacy fallback ratios
are not evidence of a published tariff. Newly registered models with unverified
prices must not be admitted for spending solely through those fallback values.

## Exclusions and preserved compatibility

| Surface omitted from suggestions | Reason and remaining work |
| --- | --- |
| Retired Gemini 2.0, ordinary 2.5 text IDs; older previews | Lifecycle-based suggestion filtering; existing registrations remain. |
| Claude 3 / 3.5 / 3.7 | [Retired](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/deprecations/partner-models); keep historical routes and configurations. Mythos IDs are not advertised without current Vertex catalog corroboration. |
| GPT-OSS 20B; older DeepSeek OCR, R1, V3.1, V3.2; Qwen3 MaaS IDs | [Retired October 21, 2026](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/deprecations/open-models); preserve legacy routes and tariff metadata. |
| imagen-4.0-generate-001; imagen-4.0-ultra-generate-001; imagen-3.0-generate-001; imagen-3.0-generate-002 | [Retired June 30, 2026](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/model-versions); remaining Fast/capability IDs lacked sufficient current lifecycle evidence. No active Imagen suggestion is invented. |
| Veo 2, Veo 3.0 and old preview aliases | [Retired/discontinued](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/model-versions); preserve routes. Lite preview's exact ID ends in -001, not a guessed -preview alias. |
| Specialized Live translation/transcription, extended-thinking aliases | Documented catalog presence alone does not qualify native relay setup and response semantics; explicit mode acceptance remains outstanding. |
| Embeddings | Existing batchEmbedContents URL/body conversion does not establish Vertex text embedding predict/instances/predictions or Embedding 2 embedContent compatibility. |
| TTS and music | Speech metadata, WAV/audio output and music operation/metering contracts are not implemented by ordinary chat conversion. |
| Omni, including current Interactions models | Interactions-only contracts cannot be advertised as generateContent/chat support. |
| Responses-only partner models | An upstream Responses endpoint is not supplied by the MaaS chat adapter; gateway Responses ingress conversion does not change this. Newly added MaaS qualification covers Chat Completions transport, not every provider-native Responses feature. |
| Other first-party, self-deployed or partner model IDs | No blind catalog copy; require exact deployment ID, transport, response, metering and lifecycle evidence. |

[Vertex text embeddings](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/embeddings/get-text-embeddings),
[multimodal embeddings](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/embeddings/get-multimodal-embeddings),
[TTS](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/text-to-speech/overview)
and [Omni](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/omni-1-2-flash)
document the distinct protocols behind these exclusions.

## Offline acceptance and limits

Catalog regressions exercise public GetModelList suggestions, exact current-ID
dispatch, sorted unique output, caller isolation and preserved legacy dispatch.
A positive current-model control accompanies retired-ID exclusion assertions.
The retired-suggestion regression failed against the previous implementation.
The Live regression verifies supported suggestions and absence of imported
Developer realtime prices; specialized modes remain explicitly unqualified.

An actual-controller alias fixture also exercises the final MaaS HTTP request, including controlled extra_body flattening and preservation of schemas, history and completion limits. Native Gemini typed payloads remain outside that OpenAI-envelope merge policy. The common response handlers retain their established canonical reasoning representation; this addition does not promise verbatim provider reasoning field names.

These checks do not establish entitlement, actual regional capacity, model quality,
paid usage reconciliation or end-to-end live Vertex acceptance. Current exact-head
test and CI results are recorded in the PR, rather than inferred from this audit.

## Official-reference recheck — 2026-11-13

The official release notes and lifecycle table corroborate Hydrogen's November 12 release, Carbon's November 10 release, and the older releases of Argon Cyber and Flash Cyber. The future dates in this audit identify retirement, minimum availability or tariff-effective dates, not model launch claims. DeepSeek's November 28 and GLM's November 30 minimum-availability dates do not imply a future release. Suggestions remain distinct from project entitlement and operational/live qualification. [Release notes](https://docs.cloud.google.com/gemini-enterprise-agent-platform/release-notes), [lifecycle](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/model-versions).

The current official pricing page explicitly lists Argon Cyber Permissive at global USD 4 input / 20 output / .20 cache read per million tokens, excluded from promotional credits. The relay's existing explicit operator-pricing admission policy is retained rather than changing configured tariffs or automatically enabling a new billing default. The earlier wording that no official tariff was verified is corrected. The page also distinguishes future Hydrogen cache-write pricing effective January 12, 2027, and describes retrievable-usage charges for Gemini 4 completions exceeding 32K tokens starting November 13, 2026. This audit does not establish live failure/cancellation invoice reconciliation. [Official pricing](https://cloud.google.com/model-garden/pricing).
