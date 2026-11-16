package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"strings"
)

// imageModelRatios contains complete image catalog entries.
var imageModelRatios = map[string]adaptor.ModelConfig{
	"gemini-3.1-flash-image-preview": {
		Ratio:                       0.50 * ratio.MilliTokensUsd,
		CompletionRatio:             3.00 / 0.50,
		Image:                       gemini31FlashImageConfig(),
		ContextLength:               131_072,
		MaxOutputTokens:             32_768,
		InputModalities:             append([]string(nil), geminiInputTextImageFile...),
		OutputModalities:            append([]string(nil), geminiOutputTextImage...),
		SupportedFeatures:           append([]string(nil), []string{"json_mode", "structured_outputs"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingImage...),
		Description:                 "Gemini 3.1 Flash Image preview has a published earliest shutdown date of June 25, 2026. Use gemini-3.1-flash-image.",
	},
	"gemini-3.1-flash-image": {
		Ratio:                       0.50 * ratio.MilliTokensUsd,
		CompletionRatio:             3.00 / 0.50,
		Image:                       gemini31FlashImageConfig(),
		ContextLength:               131_072,
		MaxOutputTokens:             32_768,
		InputModalities:             append([]string(nil), []string{"text", "image", "video", "file"}...),
		OutputModalities:            append([]string(nil), geminiOutputTextImage...),
		SupportedFeatures:           append([]string(nil), []string{"reasoning"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingImage...),
		Description:                 "Deprecated in the Gemini Developer API on 2026-10-06; no shutdown date announced. Google recommends gemini-nano-banana-2.1. That replacement requires separate gateway image-routing and billing qualification; this legacy ID is not silently remapped and retains its original tariff.",
	},
	"gemini-3.1-flash-lite-image": {
		Ratio:                       0.25 * ratio.MilliTokensUsd,
		CompletionRatio:             1.50 / 0.25,
		Image:                       geminiImageConfig(0.0336),
		ContextLength:               65_536,
		MaxOutputTokens:             4_096,
		InputModalities:             append([]string(nil), []string{"text", "image", "video", "file"}...),
		OutputModalities:            append([]string(nil), geminiOutputTextImage...),
		SupportedFeatures:           append([]string(nil), []string{"reasoning"}...),
		SupportedReasoningEfforts:   append([]string(nil), []string{"minimal", "high"}...),
		DefaultReasoningEffort:      "minimal",
		SupportedSamplingParameters: append([]string(nil), geminiSamplingImage...),
		Description:                 "Gemini 3.1 Flash Lite Image (Nano Banana 2 Lite): 65,536 input and 4,096 output tokens, text/image/video/PDF inputs, text/image outputs, 1K images only. Thinking supports minimal/high; no structured output, function calling, grounding, or caching.",
	},
	"gemini-3-pro-image-preview": {
		Ratio:            2.0 * ratio.MilliTokensUsd,
		CompletionRatio:  12.0 / 2.0,
		CachedInputRatio: 0.20 * ratio.MilliTokensUsd,
		Image:            gemini3ProImageConfig(),
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               4.0 * ratio.MilliTokensUsd,
				CompletionRatio:     18.0 / 4.0,
				CachedInputRatio:    0.40 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
		ContextLength:               65_536,
		MaxOutputTokens:             32_768,
		InputModalities:             append([]string(nil), geminiInputTextImageFile...),
		OutputModalities:            append([]string(nil), geminiOutputTextImage...),
		SupportedFeatures:           append([]string(nil), []string{"json_mode", "structured_outputs"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingImage...),
		Description:                 "Gemini 3 Pro Image preview has a published earliest shutdown date of June 25, 2026. Use gemini-3-pro-image.",
	},
	"gemini-3-pro-image": {
		Ratio:                       2.0 * ratio.MilliTokensUsd,
		CompletionRatio:             12.0 / 2.0,
		Image:                       gemini3ProImageConfig(),
		ContextLength:               65_536,
		MaxOutputTokens:             32_768,
		InputModalities:             append([]string(nil), []string{"text", "image"}...),
		OutputModalities:            append([]string(nil), geminiOutputTextImage...),
		SupportedFeatures:           append([]string(nil), []string{"reasoning"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingImage...),
		Description:                 "Gemini 3 Pro Image (Nano Banana Pro): 65,536 input and 32,768 output tokens, text/image inputs and outputs, 1K/2K/4K renders. Thinking and search grounding are available upstream; structured outputs and caching are not supported.",
	},
	"gemini-2.5-flash-image": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             2.5 / 0.30,
		Image:                       geminiImageConfig(0.039),
		ContextLength:               65_536,
		MaxOutputTokens:             32_768,
		InputModalities:             append([]string(nil), geminiInputTextImageFile...),
		OutputModalities:            append([]string(nil), geminiOutputTextImage...),
		SupportedFeatures:           append([]string(nil), []string{"json_mode", "structured_outputs"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingImage...),
		Description:                 "Gemini 2.5 Flash Image stable; published earliest shutdown March 15, 2027. Use gemini-3.1-flash-lite-image for new image workloads.",
	},
	"gemini-2.5-flash-image-preview": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             2.5 / 0.30,
		Image:                       geminiImageConfig(0.039),
		ContextLength:               65_536,
		MaxOutputTokens:             32_768,
		InputModalities:             append([]string(nil), geminiInputTextImageFile...),
		OutputModalities:            append([]string(nil), geminiOutputTextImage...),
		SupportedFeatures:           append([]string(nil), []string{"json_mode", "structured_outputs"}...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingImage...),
		Description:                 "Gemini 2.5 Flash Image preview was shut down on January 15, 2026. Prefer the current stable gemini-3.1-flash-image model.",
	},
	"gemini-2.0-flash-image": {
		Ratio:            0.15 * ratio.MilliTokensUsd,
		CompletionRatio:  0.60 / 0.15,
		CachedInputRatio: 0.0375 * ratio.MilliTokensUsd,
		Image:            geminiImageConfig(0.039),
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1.00 / 0.15,
			CompletionRatio: 0.30 / 1.00,
		},
		ContextLength:               gemini1MContext,
		MaxOutputTokens:             geminiFlashMaxOutput,
		InputModalities:             append([]string(nil), geminiInputMultimodal...),
		OutputModalities:            append([]string(nil), geminiOutputTextImage...),
		SupportedFeatures:           append([]string(nil), geminiFeatures20...),
		SupportedSamplingParameters: append([]string(nil), geminiSamplingImage...),
		Description:                 "Gemini 2.0 Flash multimodal tier with native image-generation output. (RETIRED 2026-06-01; use gemini-2.5-flash-image)",
	},
	"gemini-nano-banana-2.1": geminiNanoBanana21Config(),
}

const (
	// GeminiNanoBanana21Model is the stable upstream ID used by the Gemini and Vertex adaptors.
	GeminiNanoBanana21Model = "gemini-nano-banana-2.1"

	// Google bills image output by resolution: 1,120/1,680/3,780 tokens
	// at $30 per million output-image tokens for Standard inference.
	geminiNanoBanana21Image1KUsd = 1120 * 30.0 / 1_000_000
	geminiNanoBanana21Image2KUsd = 1680 * 30.0 / 1_000_000
	geminiNanoBanana21Image4KUsd = 3780 * 30.0 / 1_000_000
)

// geminiNanoBanana21Config builds the October 2026 Nano Banana 2.1 catalog entry.
// Parameters: none. Returns: independent standard-tier model and image metadata.
// Sources:
//   - https://ai.google.dev/gemini-api/docs/models/gemini-nano-banana-2.1
//   - https://ai.google.dev/gemini-api/docs/pricing
//   - https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/nano-banana-2-1
func geminiNanoBanana21Config() adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:           1.50 * ratio.MilliTokensUsd,
		CompletionRatio: 7.50 / 1.50,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: geminiNanoBanana21Image1KUsd,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			MinImages:        1,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"2048x2048": geminiNanoBanana21Image2KUsd / geminiNanoBanana21Image1KUsd,
				"4096x4096": geminiNanoBanana21Image4KUsd / geminiNanoBanana21Image1KUsd,
			},
		},
		ContextLength:               131_072,
		MaxOutputTokens:             32_768,
		InputModalities:             []string{"text", "image", "video", "file"},
		OutputModalities:            []string{"text", "image"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: []string{"max_tokens"},
		SupportedReasoningEfforts:   []string{"minimal", "medium", "high"},
		DefaultReasoningEffort:      "medium",
		Description: "Gemini Nano Banana 2.1 stable image generation and conversational editing model. " +
			"Supports text, images, video, and PDF inputs; text and image outputs. " +
			"128K context, 32K output, 1K default render and 2K/4K resolution tiers. " +
			"The upstream supports Google Search, but this gateway does not forward built-in web_search yet. " +
			"Requests for web_search are rejected explicitly; level-based thinking remains supported. " +
			"function calling, JSON structured output, Live API, and sampling controls are not. " +
			"Developer API caching is unavailable; Vertex AI implicit-cache pricing is provider-specific. " +
			"Higher image resolution requires explicit native imageConfig and matching billing configuration.",
	}
}

// NativeGeminiModelConfig returns transport-specific discovery metadata without changing shared pricing.
func NativeGeminiModelConfig(modelName string, cfg adaptor.ModelConfig) adaptor.ModelConfig {
	if !IsGeminiNanoBanana21Model(modelName) {
		return cfg
	}
	cfg.InputModalities = []string{"text", "image"}
	cfg.Description = strings.Replace(cfg.Description, "Supports text, images, video, and PDF inputs; text and image outputs.", "The native chat transport supports text and image inputs and outputs; video and PDF inputs are rejected.", 1)
	return cfg
}
