# Model API demo protocol audit

Reviewed: **2026-09-18**. Baseline gateway: `0932e2853c0c51d6eba3a686b975a1fbca8fcc65`.
This extends the Jev fix in PR #411. It changes documentation/example selection in the Modern frontend, not inference, routing, billing, pricing, dependencies, or CI workflows.

## Acceptance boundary

A model name, text output, or image/audio/video price does not prove an endpoint or wire format. The modal must show a reviewed gateway template or explicit uncertainty, never silently manufacture a Chat Completions request for an unknown model.

`model-api-profiles.ts` selects protocol families; `model-api-examples.ts` constructs requests and representative responses. Each displayed example carries its protocol reference and review date. The existing API Usage section remains last in desktop and mobile modals. The copy actions copy exactly the visible endpoint/request/response, and no stored user credential enters an example.

This is a **protocol-family audit**, not a claim that every deployed channel, private alias, future model version, or account has passed paid inference. The public model display DTO does not expose the mapped upstream model and effective endpoint configuration. Unknown aliases therefore show guidance. Known families receive templates for the corresponding gateway codec, conditional on the operator's actual channel configuration. Response bodies are illustrative subsets, not complete schemas or captured inference results. A catalog entry can also outlive upstream availability.

## Contract inventory

| Family | Gateway example | Decision and checked implementation |
| --- | --- | --- |
| Conventional GPT, Claude, Gemini, DeepSeek, Qwen, Llama and other recognized chat families | Chat Completions, Responses, Claude Messages | Retain the three gateway conversation encodings; do not describe these as three native APIs supported by every provider. No optional sampling defaults are guessed. |
| GPT Codex / GPT Pro / o1-pro / o3-pro | Responses | Show the provider-supported Responses entry rather than advertising native chat compatibility. |
| Jev | `POST /v1/systemone` | Preserve typed `state`/`questions`, matching `answers`, native token counters, and the previously reviewed alias response. |
| OpenAI-compatible embeddings, Jina CLIP/embeddings, Gemini embeddings, Mistral/Codestral embeddings | `POST /v1/embeddings` | Use array input; retain vector response semantics. Prices alone are not an embedding contract. Jina's gateway normalizes usage in `relay/adaptor/jina/response.go`. |
| Reranking | `POST /v1/rerank` | Query, documents and `top_n`; representative indexed relevance scores. Never derive reranking solely from per-call pricing. |
| OpenAI moderation | `POST /v1/moderations` | Moderation results, not an assistant answer. Other safety/classifier families are not automatically equivalent. |
| GLM OCR | `POST /api/paas/v4/layout_parsing` | File input and native Markdown/layout response; no unsupported public `/v1/ocr` route is invented. |
| Jina OCR | `POST /v1/chat/completions` | Include a document image and an OCR instruction. Markdown is returned inside a chat envelope; a plain text hello prompt is not an OCR example. |
| OpenAI / GLM Realtime | `GET /v1/realtime?model=...` | HTTP upgrade only; remove the unconditional OpenAI beta header. The example is not a full conversation. |
| Gemini Live / native audio | `GET /v1/realtime?model=...` | Native Gemini transport, not `/audio/speech`. Explain first-frame `setup`, then `setupComplete`, and that OpenAI application events are not interchangeable. See `relay/adaptor/gemini/live.go`. |
| Whisper / OpenAI transcription / GLM ASR | `POST /v1/audio/transcriptions` | Multipart file upload; preserve the exact selected model. GLM ASR omits the undocumented optional `response_format` parameter. The reviewed Whisper-1 profile additionally offers translation; this is not an exhaustive list of other providers' translation capabilities. |
| OpenAI speaker diarization | `POST /v1/audio/transcriptions` | `response_format=diarized_json`, `chunking_strategy=auto`, speaker-bearing segments. The gateway forwards these multipart fields. |
| OpenAI speech / GLM TTS | `POST /v1/audio/speech` | Use `alloy` or GLM's documented `tongtong`; show WAV bytes saved to a file, not JSON. |
| GLM voice cloning | `POST /v1/voice/clones` | Preserve the gateway's separate clone DTO and voice/file/request-ID response. The primary provider reference could not be fetched during this audit; the UI reference explicitly pins the reviewed gateway adaptor instead. Confirm provider file IDs and upstream availability. |
| Sora | `POST /v1/videos` | Add `seconds: "4"` and `size: "1280x720"`. Duration is required by per-second admission in `relay/controller/video.go`. Sora job IDs, status polling and content download are not universal video-provider contracts. |
| GPT Image | Image generation and multipart image editing | Use the same actual model ID for both tasks; omit legacy `response_format`; show base64. Only valid pixel sizes may be copied from pricing metadata. |
| DALL-E | Generation; DALL-E 2 also editing | Explicit base64 response format. Do not invent DALL-E 3 editing support. Availability of legacy IDs remains provider-dependent. |
| CogView | Image generation | The Zhipu converter sends model/prompt and returns a URL; do not display base64 or parameters discarded by that converter. |
| Grok image | Image generation | Explicit `b64_json`, matching xAI's documented response. Recognize this family without relying on a pricing object. |
| Legacy completions | `POST /v1/completions` | Prompt and completion-shaped output, with an availability warning. |
| Gemini native TTS/image, Imagen, Veo, Grok video, Mistral OCR, Qwen image editing, other unreviewed native tasks | Guidance and a reference where verified | Do not substitute a generic OpenAI request when the current gateway conversion/transport has not been established. Adding support requires an explicit protocol profile and evidence, not a regex based solely on media output. |
| Unknown aliases; conflicting explicit task features | Guidance, no executable request | No fabricated endpoint, response or copy action. The operator must resolve the actual model and enabled endpoint. |

## Primary references

Protocol references are centralized beside the resolver. Particularly important source checks were:

- [TypeSafe API](https://docs.typesafe.ai/api) and [model aliases](https://docs.typesafe.ai/models).
- [OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create), [Responses](https://developers.openai.com/api/docs/guides/migrate-to-responses), and [Claude Messages](https://platform.claude.com/docs/en/api/typescript/messages/create).
- [OpenAI transcription and diarization](https://developers.openai.com/api/docs/guides/speech-to-text), [WebSockets](https://developers.openai.com/api/docs/guides/voice-websockets), [images](https://developers.openai.com/api/reference/resources/images/methods/generate), [image editing](https://developers.openai.com/api/reference/resources/images/methods/edit), and [video generation](https://developers.openai.com/api/docs/guides/video-generation).
- [Jina OCR's actual image-bearing request](https://jina.ai/news/jina-ocr-v1-faster-document-parsing-on-low-budget-gpus), [Jina search APIs](https://api.jina.ai/docs), [Cohere reranking](https://docs.cohere.com/v2/reference/rerank), and [Mistral embeddings](https://docs.mistral.ai/api/endpoint/embeddings).
- [Gemini Live](https://ai.google.dev/gemini-api/docs/live-api), [native speech generation](https://ai.google.dev/gemini-api/docs/speech-generation), and [native image generation](https://ai.google.dev/gemini-api/docs/image-generation).
- [GLM ASR](https://docs.z.ai/api-reference/audio/audio-transcriptions), [GLM OCR](https://docs.z.ai/api-reference/tools/layout-parsing), [GLM TTS](https://docs.bigmodel.cn/api-reference/模型-api/文本转语音), and [CogView](https://docs.bigmodel.cn/api-reference/模型-api/图像生成).
- [xAI images](https://docs.x.ai/developers/model-capabilities/images/generation) and [xAI videos](https://docs.x.ai/developers/model-capabilities/video/generation): the latter uses duration/request IDs and is not the Sora schema.

## Regression evidence and maintenance

The original Jev-only generator was reconstructed and checked against Git blob `affae8e573e39024be474490599ab027b9a6f3c5`. Before changes, the new audit suite had **24 failing cases and one passing ordinary-model control**. These cover both concrete payload mistakes and the new requirement to avoid unverified fallbacks; they are not 24 independent live provider incidents. The same assertions passed after implementation.

Local verification: **100 generator/profile tests pass**, with production generator/resolver passing standalone strict TypeScript checking. Because the local container cannot install the repository dependencies, the committed Vitest assertions were transpiled to a temporary copy and executed with Node's test runner by substituting only the runner import. This does not establish a local React/Vitest or production-build pass. Desktop/mobile guidance, transitions, exact copy actions, source links, localization and bottom placement are additionally tested through the real pricing modal in the existing frontend CI. No test workflow was added or bypassed.

To maintain the contract:

1. Verify the actual gateway route, request converter, mandatory fields, response normalization and transport alongside a primary API source.
2. Add a specific task profile before broad family handling; do not use pricing as endpoint evidence. Keep unrelated unknown models unverified.
3. Add positive and negative fixtures, execute curl through the network-free shell harness, and test actual modal rendering/copy actions when introducing a format or empty state.
4. Update references, review date and all five locale files. Run the existing Modern tests and production build before merge.

No production deployment or paid API calls were performed for this audit.
