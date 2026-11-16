package geminiOpenaiCompatible

// Gemini metadata sources
//   - Gemini family: https://ai.google.dev/gemini-api/docs/models
//   - Gemma family: https://ai.google.dev/gemma/docs/ and HuggingFace cards under https://huggingface.co/google
//
// Pricing fields (Ratio, CompletionRatio, Cached*, Tiers, Image, Audio, Embedding) are intentionally
// untouched here; see ModelRatios for those. We only annotate context length, modality, capability,
// sampling, quantization, HuggingFace identity, and short descriptions for OpenRouter discovery.

const (
	// gemini1MContext advertises the published 1M token context window for Gemini 1.5/2.x/3.x mainline tiers.
	gemini1MContext int32 = 1_048_576
	// gemini2MContext advertises the 2M token context window Google offers for select Pro tiers.
	gemini2MContext int32 = 2_097_152
	// geminiProMaxOutput is the standard maximum output token length advertised for Pro tiers (2.5+).
	geminiProMaxOutput int32 = 65536
	// geminiFlashMaxOutput is the standard maximum output token length advertised for Gemini 2.x Flash tiers.
	geminiFlashMaxOutput int32 = 8192
	// gemini3FlashMaxOutput is the documented maximum output token length for Gemini 3.x Flash tiers.
	gemini3FlashMaxOutput int32 = 65536
)

// geminiInputTextImageFile lists the modalities every multimodal Gemini chat tier accepts.
var geminiInputTextImageFile = []string{"text", "image", "file"}

// geminiInputMultimodal lists the full multimodal input set advertised for Gemini 2.5+/3.x chat tiers
// per https://ai.google.dev/gemini-api/docs/models (text, image, audio, video, and arbitrary files).
var geminiInputMultimodal = []string{"text", "image", "audio", "video", "file"}

// geminiInputLive lists the Live API audio/text/image inputs per https://ai.google.dev/gemini-api/docs/live.
var geminiInputLive = []string{"text", "audio", "image"}

// geminiInputTextOnly lists modalities for text-only models (Gemma, embedders, AQA, Imagen prompt).
var geminiInputTextOnly = []string{"text"}

// geminiOutputText lists the default text-only output modality set.
var geminiOutputText = []string{"text"}

// geminiOutputTextImage lists output modalities for native image-generation tiers.
var geminiOutputTextImage = []string{"text", "image"}

// geminiOutputTextAudio lists output modalities for Live API tiers that stream audio alongside text.
var geminiOutputTextAudio = []string{"text", "audio"}

// geminiOutputAudio lists output modalities for TTS-only tiers per https://ai.google.dev/gemini-api/docs/models.
var geminiOutputAudio = []string{"audio"}

// geminiOutputImage lists output modalities for image-only generation models (Imagen).
var geminiOutputImage = []string{"image"}

// geminiOutputTextVideo lists output modalities for unified tiers that stream text alongside native video.
var geminiOutputTextVideo = []string{"text", "video"}

// geminiFeatures25Plus advertises capabilities for Gemini 2.5+ tiers (with thinking/reasoning).
var geminiFeatures25Plus = []string{"tools", "json_mode", "structured_outputs", "web_search", "reasoning"}

// geminiFeatures20 advertises capabilities for Gemini 2.0 tiers (no reasoning toggle exposed in public API).
var geminiFeatures20 = []string{"tools", "json_mode", "structured_outputs", "web_search"}

// geminiFeaturesGemma advertises the limited capability surface Gemma tiers expose via the Gemini API.
var geminiFeaturesGemma = []string{"json_mode"}

// geminiSamplingChat lists the sampling parameters Google documents for Gemini chat completions.
var geminiSamplingChat = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}

// Reasoning effort enumerations published for the Gemini OpenAI-compatible endpoint
// (https://ai.google.dev/gemini-api/docs/openai). Gemini 3.x exposes minimal/low/medium/high mapping
// to thinkingLevel; Gemini 2.5 maps the same labels to thinkingBudget tiers. The "none" value is only
// honored on 2.5 models that allow disabling thought, and the Pro tiers cannot disable reasoning.
var (
	gemini25ProReasoningEfforts      = []string{"minimal", "low", "medium", "high"}
	gemini25ReasoningEfforts         = []string{"minimal", "low", "medium", "high", "none"}
	gemini3ProReasoningEfforts       = []string{"low", "medium", "high"}
	gemini3FlashReasoningEfforts     = []string{"minimal", "low", "medium", "high"}
	gemini3FlashLiteReasoningEfforts = []string{"minimal", "low", "medium", "high"}
)

// Published thinking-budget ceilings from https://ai.google.dev/gemini-api/docs/thinking.
// Gemini 3.x uses thinkingLevel rather than an integer budget; we surface the per-level cap (24,576)
// because the OpenAI-compat layer continues to translate "high" into a budget request internally.
const (
	gemini25ProMaxThinkingBudget       int32 = 32_768
	gemini25FlashMaxThinkingBudget     int32 = 24_576
	gemini25FlashLiteMaxThinkingBudget int32 = 24_576
	gemini3LevelMaxThinkingBudget      int32 = 24_576
)

// geminiSamplingGemma lists the sampling parameters surfaced by Gemini-served Gemma tiers.
var geminiSamplingGemma = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}

// geminiSamplingImage lists the sampling parameters relevant to image-generation tiers.
var geminiSamplingImage = []string{"max_tokens"}
