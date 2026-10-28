# Google Gemini model catalog refresh — October 28, 2026

This change updates the shared Google Gemini model metadata consumed by the
Gemini Developer API, Gemini OpenAI-compatible, and Vertex AI adaptors. It
focuses on the currently missing **Gemini Nano Banana 2.1** model and Google's
latest published lifecycle dates; it does not remove any configured model IDs.

## Newly registered: `gemini-nano-banana-2.1`

Google released the stable image generation and conversational editing model
on October 6, 2026. The model supports `generateContent` on the Gemini
Developer API and the global Vertex AI endpoint.

| Attribute | Verified Standard-tier value |
| --- | --- |
| Model ID | `gemini-nano-banana-2.1` |
| Input | Text, image, video, PDF/file |
| Output | Text and image |
| Context / output limit | 131,072 / 32,768 tokens |
| Input cost | $1.50 per 1M tokens |
| Text/thinking output | $7.50 per 1M tokens |
| Image output | $30 per 1M image tokens |
| 1K / 2K / 4K image render | $0.0336 / $0.0504 / $0.1134 |
| Default render tier | 1K; **no 512-pixel tier** |
| Grounded search | $14 per 1,000 calls after upstream allowance |
| Thinking levels | `minimal`, `medium` (default), `high` |
| Not supported | Temperature/top-p/top-k, function calling, structured outputs, Live |
| Caching | Gemini Developer API: unsupported; Vertex AI: implicit cached input at $0.15/1M tokens |

The shared Gemini catalog advertises Standard Developer API token and image
prices without a cached-input discount. Vertex applies its **own**
cached-input override without mutating that shared entry. All models already
registered in the catalog retain their previous tariffs.

### Important compatibility and billing limits

- The native Gemini request converter explicitly sets
  `responseModalities: ["TEXT", "IMAGE"]` for the named Nano Banana model,
  removes unsupported `temperature` and `topP`, and preserves supported
  system instructions. The existing numeric-version cutoff behavior remains
  unchanged for older Gemini models.
- Vertex routes the named model to `locations/global`. Its nonnumeric
  model ID would otherwise fall through the existing version-based route.
- The current **OpenAI Chat Completions** interface has no upstream
  `imageConfig.imageSize` selector. It therefore uses Google's default
  **1K** render tier when estimating output-image quota. The 2K and 4K rates
  are included in the image pricing metadata for channels that can explicitly
  select and consistently bill those resolutions. Merely choosing a billing
  size override does not make an upstream 2K/4K request.
- This catalog registration does **not** implement full compatibility with
  Gemini's Interactions API, image-edit/multipart endpoints, configurable
  aspect ratios, or a native image-generation endpoint. It also does not
  reinterpret Gemini's image output token receipt; the existing chat image
  billing path remains in effect.
- Google AI Studio / Vertex permissions, quota and region availability still
  apply. No paid upstream requests were issued as part of this refresh.

## Lifecycle descriptions updated from Google's October 22 deprecation page

| Model | Published date | Recommended migration |
| --- | --- | --- |
| `gemini-3.1-flash-tts-preview` | Earliest November 17, 2026 | 3.8 Flash TTS / 3.8 Flash-Lite TTS |
| `gemini-2.5-flash-preview-tts` | Earliest November 17, 2026 | 3.8 Flash-Lite TTS |
| `gemini-2.5-pro-preview-tts` | Earliest November 17, 2026 | 3.8 Flash TTS |
| `gemini-3.1-flash-live-preview` | Earliest November 17, 2026 | 3.8 Live |
| `gemini-2.5-flash-native-audio-preview-12-2025` | Earliest November 17, 2026 | 3.8 Live |
| `gemini-2.5-flash-image` | Earliest March 15, 2027 | 3.1 Flash Lite Image |
| `gemini-omni-flash-preview` | October 22, 2026 | Omni 1.1 Flash |
| `gemini-2.5-computer-use-preview-10-2025` | July 28, 2026 | 3.8 Flash |

The current deprecation table supersedes older dates retained in historical
September audit files. It also explicitly states that the `gemini-2.5-pro`,
`gemini-2.5-flash`, and `gemini-2.5-flash-lite` models are **not deprecated**
for existing users; this change leaves their pricing and status untouched.
Upstream dates may be *earliest possible* shutdown dates rather than an
assurance of service on a particular day.

## Regression checks

Focused tests cover shared discovery, model caps, supported features,
independent pricing maps, the absence of a 512-pixel tier, three image prices,
grounded-search billing, native payload conversion and image responses, Vertex
global routing and cached-input pricing, default rendered image quota, and
backward-compatible lifecycle pricing. Existing September and October
lifecycle test assertions are updated for the current official schedule.

Suggested local verification with the repository's Go toolchain:

```bash
go test ./relay/adaptor/geminiOpenaiCompatible ./relay/adaptor/gemini ./relay/adaptor/vertexai ./relay/controller
```

No CI workflow changes are needed.

## Primary sources

1. [Gemini Nano Banana 2.1 model card](https://ai.google.dev/gemini-api/docs/models/gemini-nano-banana-2.1) (updated October 6, 2026)
2. [Gemini API pricing](https://ai.google.dev/gemini-api/docs/pricing) (image token consumption and Standard tariffs)
3. [Gemini Generate Content image generation](https://ai.google.dev/gemini-api/docs/generate-content/image-generation) (native REST example)
4. [Vertex Nano Banana 2.1 model specifications](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/nano-banana-2-1) (global region and supported parameters)
5. [Google Cloud Agent Platform pricing](https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing) (cached-input price)
6. [Gemini model deprecations](https://ai.google.dev/gemini-api/docs/deprecations) (updated October 22, 2026)
7. [Gemini API release notes](https://ai.google.dev/gemini-api/docs/changelog) (October 6/8/13 announcements)
