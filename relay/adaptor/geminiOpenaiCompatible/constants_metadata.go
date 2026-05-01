package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

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

// geminiInputTextOnly lists modalities for text-only models (Gemma, embedders, AQA, Imagen prompt).
var geminiInputTextOnly = []string{"text"}

// geminiOutputText lists the default text-only output modality set.
var geminiOutputText = []string{"text"}

// geminiOutputTextImage lists output modalities for native image-generation tiers.
var geminiOutputTextImage = []string{"text", "image"}

// geminiOutputImage lists output modalities for image-only generation models (Imagen).
var geminiOutputImage = []string{"image"}

// geminiFeatures25Plus advertises capabilities for Gemini 2.5+ tiers (with thinking/reasoning).
var geminiFeatures25Plus = []string{"tools", "json_mode", "structured_outputs", "web_search", "reasoning"}

// geminiFeatures20 advertises capabilities for Gemini 2.0 tiers (no reasoning toggle exposed in public API).
var geminiFeatures20 = []string{"tools", "json_mode", "structured_outputs", "web_search"}

// geminiFeaturesGemma advertises the limited capability surface Gemma tiers expose via the Gemini API.
var geminiFeaturesGemma = []string{"json_mode"}

// geminiSamplingChat lists the sampling parameters Google documents for Gemini chat completions.
var geminiSamplingChat = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}

// geminiSamplingGemma lists the sampling parameters surfaced by Gemini-served Gemma tiers.
var geminiSamplingGemma = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}

// geminiSamplingImage lists the sampling parameters relevant to image-generation tiers.
var geminiSamplingImage = []string{"max_tokens"}

// geminiMetadataOverrides supplies per-model metadata that augments ModelRatios at package init.
// Keys must match ModelRatios. Pricing-related fields here are ignored by mergeGeminiMetadata.
var geminiMetadataOverrides = map[string]adaptor.ModelConfig{
	// Gemma Models (open-weight).
	"gemma-2-2b-it": {
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             geminiInputTextOnly,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeaturesGemma,
		SupportedSamplingParameters: geminiSamplingGemma,
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-2-2b-it",
		Description:                 "Gemma 2 2B instruction-tuned open-weight model served via the Gemini API.",
	},
	"gemma-2-9b-it": {
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             geminiInputTextOnly,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeaturesGemma,
		SupportedSamplingParameters: geminiSamplingGemma,
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-2-9b-it",
		Description:                 "Gemma 2 9B instruction-tuned open-weight model served via the Gemini API.",
	},
	"gemma-2-27b-it": {
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             geminiInputTextOnly,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeaturesGemma,
		SupportedSamplingParameters: geminiSamplingGemma,
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-2-27b-it",
		Description:                 "Gemma 2 27B instruction-tuned open-weight model served via the Gemini API.",
	},
	"gemma-3-27b-it": {
		ContextLength:               128_000,
		MaxOutputTokens:             8192,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeaturesGemma,
		SupportedSamplingParameters: geminiSamplingGemma,
		Quantization:                "bf16",
		HuggingFaceID:               "google/gemma-3-27b-it",
		Description:                 "Gemma 3 27B instruction-tuned open-weight multimodal model served via the Gemini API.",
	},

	// Embedding & evaluation models.
	"gemini-embedding-001": {
		ContextLength:    2048,
		MaxOutputTokens:  0,
		InputModalities:  geminiInputTextOnly,
		OutputModalities: geminiOutputText,
		Description:      "Legacy text-only embedding model (gemini-embedding-001).",
	},
	"gemini-embedding-2-preview": {
		ContextLength:    8192,
		MaxOutputTokens:  0,
		InputModalities:  geminiInputTextImageFile,
		OutputModalities: geminiOutputText,
		Description:      "Multimodal embedding preview model accepting text, image, audio, and video inputs.",
	},
	"aqa": {
		ContextLength:               7168,
		MaxOutputTokens:             1024,
		InputModalities:             geminiInputTextOnly,
		OutputModalities:            geminiOutputText,
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "max_tokens"},
		Description:                 "Attributed Question Answering (AQA) grounding model.",
	},

	// Gemini 3.x family.
	"gemini-3.1-pro-preview": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 3.1 Pro preview multimodal reasoning tier with 1M context.",
	},
	"gemini-3.1-pro-preview-customtools": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 3.1 Pro preview tier configured for custom tool definitions.",
	},
	"gemini-3.1-flash-image-preview": {
		ContextLength:               131_072,
		MaxOutputTokens:             32_768,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputTextImage,
		SupportedFeatures:           []string{"json_mode", "structured_outputs"},
		SupportedSamplingParameters: geminiSamplingImage,
		Description:                 "Gemini 3.1 Flash native image-generation preview tier with up to 128K context.",
	},
	"gemini-3.1-flash-live-preview": {
		ContextLength:               32_768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text", "image", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode"},
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 3.1 Flash Live preview tier optimized for low-latency bidirectional sessions.",
	},
	"gemini-3.1-flash-lite-preview": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             gemini3FlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 3.1 Flash Lite preview tier for cost-efficient high-throughput workloads.",
	},
	"gemini-3-pro-preview": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 3 Pro preview multimodal reasoning tier with 1M context.",
	},
	"gemini-3-flash-preview": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             gemini3FlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 3 Flash preview multimodal model balancing latency and quality.",
	},
	"gemini-3-pro-image-preview": {
		ContextLength:               65_536,
		MaxOutputTokens:             32_768,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputTextImage,
		SupportedFeatures:           []string{"json_mode", "structured_outputs"},
		SupportedSamplingParameters: geminiSamplingImage,
		Description:                 "Gemini 3 Pro native image-generation preview tier with 1K/2K/4K rendering.",
	},

	// Gemini 2.5 Pro & Computer Use Models.
	"gemini-2.5-pro": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Pro multimodal reasoning tier (1M context, thinking enabled).",
	},
	"gemini-2.5-pro-preview": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Pro preview tier (1M context, thinking enabled).",
	},
	"gemini-2.5-computer-use-preview": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Computer Use preview tier with browser/agent control tools.",
	},
	"gemini-2.5-computer-use-preview-10-2025": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiProMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Computer Use preview snapshot dated 10-2025.",
	},

	// Gemini 2.5 Flash family.
	"gemini-2.5-flash": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Flash multimodal tier optimized for latency and cost.",
	},
	"gemini-2.5-flash-preview": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Flash preview tier.",
	},
	"gemini-2.5-flash-preview-09-2025": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Flash preview snapshot dated 09-2025.",
	},
	"gemini-2.5-flash-lite": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Flash Lite multimodal tier for cost-sensitive workloads.",
	},
	"gemini-2.5-flash-lite-preview": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Flash Lite preview tier.",
	},
	"gemini-2.5-flash-lite-preview-09-2025": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures25Plus,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Flash Lite preview snapshot dated 09-2025.",
	},
	"gemini-2.5-flash-native-audio": {
		ContextLength:               128_000,
		MaxOutputTokens:             8192,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Flash native-audio dialog tier with low-latency audio in/out.",
	},
	"gemini-2.5-flash-native-audio-preview-09-2025": {
		ContextLength:               128_000,
		MaxOutputTokens:             8192,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Flash native-audio preview snapshot dated 09-2025.",
	},
	"gemini-2.5-flash-native-audio-preview-12-2025": {
		ContextLength:               128_000,
		MaxOutputTokens:             8192,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.5 Flash native-audio preview snapshot dated 12-2025.",
	},
	"gemini-2.5-flash-image": {
		ContextLength:               65_536,
		MaxOutputTokens:             32_768,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputTextImage,
		SupportedFeatures:           []string{"json_mode", "structured_outputs"},
		SupportedSamplingParameters: geminiSamplingImage,
		Description:                 "Gemini 2.5 Flash native image-generation tier (Nano Banana family).",
	},
	"gemini-2.5-flash-image-preview": {
		ContextLength:               65_536,
		MaxOutputTokens:             32_768,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputTextImage,
		SupportedFeatures:           []string{"json_mode", "structured_outputs"},
		SupportedSamplingParameters: geminiSamplingImage,
		Description:                 "Gemini 2.5 Flash native image-generation preview tier (Nano Banana preview).",
	},
	"gemini-2.5-flash-preview-tts": {
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             geminiInputTextOnly,
		OutputModalities:            geminiOutputText,
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k"},
		Description:                 "Gemini 2.5 Flash preview text-to-speech tier (audio output billed via Audio config).",
	},
	"gemini-2.5-pro-preview-tts": {
		ContextLength:               8192,
		MaxOutputTokens:             8192,
		InputModalities:             geminiInputTextOnly,
		OutputModalities:            geminiOutputText,
		SupportedSamplingParameters: []string{"temperature", "top_p", "top_k"},
		Description:                 "Gemini 2.5 Pro preview text-to-speech tier (audio output billed via Audio config).",
	},
	"gemini-robotics-er-1.5-preview": {
		ContextLength:               1_000_000,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini Robotics-ER 1.5 preview tier for embodied reasoning workloads.",
	},

	// Gemini 2.0 Flash Models.
	"gemini-2.0-flash": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures20,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.0 Flash multimodal tier with 1M context.",
	},
	"gemini-2.0-flash-image": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputTextImage,
		SupportedFeatures:           geminiFeatures20,
		SupportedSamplingParameters: geminiSamplingImage,
		Description:                 "Gemini 2.0 Flash multimodal tier with native image-generation output.",
	},
	"gemini-2.0-flash-lite": {
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             geminiInputTextImageFile,
		OutputModalities:            geminiOutputText,
		SupportedFeatures:           geminiFeatures20,
		SupportedSamplingParameters: geminiSamplingChat,
		Description:                 "Gemini 2.0 Flash Lite cost-optimized multimodal tier.",
	},
}

// mergeGeminiMetadata copies non-pricing metadata fields from override into base.
// Pricing-related fields (Ratio, CompletionRatio, Cached*, CacheWrite*, Tiers, MaxTokens,
// Video, Audio, Image, Embedding) are preserved from base. The function returns base unchanged
// when override has no metadata to apply.
func mergeGeminiMetadata(base, override adaptor.ModelConfig) adaptor.ModelConfig {
	if override.ContextLength != 0 {
		base.ContextLength = override.ContextLength
	}
	if override.MaxOutputTokens != 0 {
		base.MaxOutputTokens = override.MaxOutputTokens
	}
	if len(override.InputModalities) > 0 {
		base.InputModalities = append([]string(nil), override.InputModalities...)
	}
	if len(override.OutputModalities) > 0 {
		base.OutputModalities = append([]string(nil), override.OutputModalities...)
	}
	if len(override.SupportedFeatures) > 0 {
		base.SupportedFeatures = append([]string(nil), override.SupportedFeatures...)
	}
	if len(override.SupportedSamplingParameters) > 0 {
		base.SupportedSamplingParameters = append([]string(nil), override.SupportedSamplingParameters...)
	}
	if override.Quantization != "" {
		base.Quantization = override.Quantization
	}
	if override.HuggingFaceID != "" {
		base.HuggingFaceID = override.HuggingFaceID
	}
	if override.Description != "" {
		base.Description = override.Description
	}
	return base
}

// init applies geminiMetadataOverrides to ModelRatios so downstream consumers (OpenRouter
// provider mapping, channel introspection) observe the enriched metadata without changing
// the price table or losing entries that lack metadata overrides.
func init() {
	for name, override := range geminiMetadataOverrides {
		base, ok := ModelRatios[name]
		if !ok {
			continue
		}
		ModelRatios[name] = mergeGeminiMetadata(base, override)
	}
}
