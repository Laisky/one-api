package geminiOpenaiCompatible

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// geminiImageConfig returns the baseline image metadata for Gemini image generation models.
func geminiImageConfig(pricePerImage float64) *adaptor.ImagePricingConfig {
	return &adaptor.ImagePricingConfig{
		PricePerImageUsd: pricePerImage,
		DefaultSize:      "1024x1024",
		DefaultQuality:   "standard",
		MinImages:        1,
		SizeMultipliers: map[string]float64{
			"1024x1024": 1,
		},
	}
}

// gemini3ProImageConfig encodes the multi-tier pricing Google published for Gemini 3 Pro Image Preview.
// 1K/2K renders cost $0.134 per image, while 4K outputs are billed at $0.24 per image.
const (
	gemini3ProImageBasePrice = 0.134
	gemini3ProImage4KPrice   = 0.24
)

func gemini3ProImageConfig() *adaptor.ImagePricingConfig {
	return &adaptor.ImagePricingConfig{
		PricePerImageUsd: gemini3ProImageBasePrice,
		DefaultSize:      "1024x1024",
		DefaultQuality:   "standard",
		MinImages:        1,
		SizeMultipliers: map[string]float64{
			"1024x1024": 1,
			"2048x2048": 1,
			"4096x4096": gemini3ProImage4KPrice / gemini3ProImageBasePrice,
		},
	}
}

var (
	gemini25ProPricing = adaptor.ModelConfig{
		Ratio:             1.25 * ratio.MilliTokensUsd,
		CompletionRatio:   10.0 / 1.25,
		CacheWrite5mRatio: 0.125 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.125 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               2.50 * ratio.MilliTokensUsd,
				CompletionRatio:     15.0 / 2.50,
				CacheWrite5mRatio:   0.25 * ratio.MilliTokensUsd,
				CacheWrite1hRatio:   0.25 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
	}
	gemini25FlashPricing = adaptor.ModelConfig{
		Ratio:             0.30 * ratio.MilliTokensUsd,
		CompletionRatio:   2.50 / 0.30,
		CacheWrite5mRatio: 0.03 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.03 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1.00 / 0.30,
			CompletionRatio: 0.30,
		},
	}
	gemini25FlashLitePricing = adaptor.ModelConfig{
		Ratio:             0.10 * ratio.MilliTokensUsd,
		CompletionRatio:   0.40 / 0.10,
		CacheWrite5mRatio: 0.01 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.01 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     0.30 / 0.10,
			CompletionRatio: 0.10 / 0.30,
		},
	}
	gemini3FlashPricing = adaptor.ModelConfig{
		Ratio:             0.50 * ratio.MilliTokensUsd,
		CompletionRatio:   3.00 / 0.50,
		CacheWrite5mRatio: 0.05 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.05 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1.00 / 0.50,
			CompletionRatio: 3.00 / 1.00,
		},
	}
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on Google AI pricing: https://ai.google.dev/pricing
//
// ⚠️ Note: should also check relay/adaptor/vertexai/adaptor.go:IsRequireGlobalEndpoint
var ModelRatios = map[string]adaptor.ModelConfig{
	// Gemma Models
	"gemma-2-2b-it":  {Ratio: 0.35 * ratio.MilliTokensUsd, CompletionRatio: 1.4},
	"gemma-2-9b-it":  {Ratio: 0.35 * ratio.MilliTokensUsd, CompletionRatio: 1.4},
	"gemma-2-27b-it": {Ratio: 0.35 * ratio.MilliTokensUsd, CompletionRatio: 1.4},
	"gemma-3-27b-it": {Ratio: 0.35 * ratio.MilliTokensUsd, CompletionRatio: 1.4},

	// Embedding & evaluation models
	"gemini-embedding-001": {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1},
	"aqa":                  {Ratio: 1, CompletionRatio: 1},

	// Gemini 3 Models
	"gemini-3.1-pro-preview": {
		Ratio:             2.0 * ratio.MilliTokensUsd,
		CompletionRatio:   12.0 / 2.0,
		CacheWrite5mRatio: 0.20 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.20 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               4.0 * ratio.MilliTokensUsd,
				CompletionRatio:     18.0 / 4.0,
				CacheWrite5mRatio:   0.40 * ratio.MilliTokensUsd,
				CacheWrite1hRatio:   0.40 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
	},
	"gemini-3.1-pro-preview-customtools": {
		Ratio:             2.0 * ratio.MilliTokensUsd,
		CompletionRatio:   12.0 / 2.0,
		CacheWrite5mRatio: 0.20 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.20 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               4.0 * ratio.MilliTokensUsd,
				CompletionRatio:     18.0 / 4.0,
				CacheWrite5mRatio:   0.40 * ratio.MilliTokensUsd,
				CacheWrite1hRatio:   0.40 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
	},
	"gemini-3-pro-preview": {
		Ratio:             2.0 * ratio.MilliTokensUsd,
		CompletionRatio:   12.0 / 2.0,
		CacheWrite5mRatio: 0.20 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.20 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               4.0 * ratio.MilliTokensUsd,
				CompletionRatio:     18.0 / 4.0,
				CacheWrite5mRatio:   0.40 * ratio.MilliTokensUsd,
				CacheWrite1hRatio:   0.40 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
	},
	"gemini-3-flash-preview": gemini3FlashPricing,
	"gemini-3-pro-image-preview": {
		Ratio:             2.0 * ratio.MilliTokensUsd,
		CompletionRatio:   12.0 / 2.0,
		CacheWrite5mRatio: 0.20 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.20 * ratio.MilliTokensUsd,
		Image:             gemini3ProImageConfig(),
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               4.0 * ratio.MilliTokensUsd,
				CompletionRatio:     18.0 / 4.0,
				CacheWrite5mRatio:   0.40 * ratio.MilliTokensUsd,
				CacheWrite1hRatio:   0.40 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
	},

	// Gemini 2.5 Pro & Computer Use Models
	"gemini-2.5-pro":                          gemini25ProPricing,
	"gemini-2.5-pro-preview":                  gemini25ProPricing,
	"gemini-2.5-computer-use-preview":         gemini25ProPricing,
	"gemini-2.5-computer-use-preview-10-2025": gemini25ProPricing,

	// Gemini 2.5 Flash Family
	"gemini-2.5-flash":                      gemini25FlashPricing,
	"gemini-2.5-flash-preview":              gemini25FlashPricing,
	"gemini-2.5-flash-preview-09-2025":      gemini25FlashPricing,
	"gemini-2.5-flash-lite":                 gemini25FlashLitePricing,
	"gemini-2.5-flash-lite-preview":         gemini25FlashLitePricing,
	"gemini-2.5-flash-lite-preview-09-2025": gemini25FlashLitePricing,
	"gemini-2.5-flash-native-audio": {
		Ratio:           0.50 * ratio.MilliTokensUsd,
		CompletionRatio: 2.0 / 0.50,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     3.0 / 0.50,
			CompletionRatio: 12.0 / 2.0,
		},
	},
	"gemini-2.5-flash-native-audio-preview-09-2025": {
		Ratio:           0.50 * ratio.MilliTokensUsd,
		CompletionRatio: 2.0 / 0.50,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     3.0 / 0.50,
			CompletionRatio: 12.0 / 2.0,
		},
	},
	"gemini-2.5-flash-native-audio-preview-12-2025": {
		Ratio:           0.50 * ratio.MilliTokensUsd,
		CompletionRatio: 2.0 / 0.50,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     3.0 / 0.50,
			CompletionRatio: 12.0 / 2.0,
		},
	},
	"gemini-2.5-flash-image":         {Ratio: 0.30 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 0.30, Image: geminiImageConfig(0.039)},
	"gemini-2.5-flash-image-preview": {Ratio: 0.30 * ratio.MilliTokensUsd, CompletionRatio: 2.5 / 0.30, Image: geminiImageConfig(0.039)},
	"gemini-2.5-flash-preview-tts": {
		Ratio:           0.50 * ratio.MilliTokensUsd,
		CompletionRatio: 10.0 / 0.50,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1,
			CompletionRatio: 1,
		},
	},
	"gemini-2.5-pro-preview-tts": {
		Ratio:           1.0 * ratio.MilliTokensUsd,
		CompletionRatio: 20.0 / 1.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1,
			CompletionRatio: 1,
		},
	},
	"gemini-robotics-er-1.5-preview": {
		Ratio:           0.30 * ratio.MilliTokensUsd,
		CompletionRatio: 2.5 / 0.30,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1.00 / 0.30,
			CompletionRatio: 0.30,
		},
	},

	// Gemini 2.0 Flash Models
	"gemini-2.0-flash": {
		Ratio:             0.10 * ratio.MilliTokensUsd,
		CompletionRatio:   0.40 / 0.10,
		CacheWrite5mRatio: 0.025 * ratio.MilliTokensUsd,
		CacheWrite1hRatio: 0.025 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     0.70 / 0.10,
			CompletionRatio: 0.10 / 0.70,
		},
	},
	"gemini-2.0-flash-lite": {Ratio: 0.075 * ratio.MilliTokensUsd, CompletionRatio: 0.30 / 0.075},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

const geminiWebSearchUsdPerCall = 35.0 / 1000.0

// geminiWebSearchModels enumerates Gemini models with grounded web search pricing in Google documentation.
// Source: https://ai.google.dev/gemini-api/docs/pricing (retrieved via https://r.jina.ai/https://ai.google.dev/gemini-api/docs/pricing)
var geminiWebSearchModels = map[string]struct{}{
	"gemini-3.1-pro-preview":                  {},
	"gemini-3.1-pro-preview-customtools":      {},
	"gemini-3-pro-preview":                    {},
	"gemini-3-flash-preview":                  {},
	"gemini-2.5-pro":                          {},
	"gemini-2.5-pro-preview":                  {},
	"gemini-2.5-computer-use-preview":         {},
	"gemini-2.5-computer-use-preview-10-2025": {},
	"gemini-2.5-flash":                        {},
	"gemini-2.5-flash-preview":                {},
	"gemini-2.5-flash-lite":                   {},
	"gemini-2.5-flash-lite-preview":           {},
	"gemini-2.0-flash":                        {},
	"gemini-2.0-flash-lite":                   {},
	"gemini-robotics-er-1.5-preview":          {},
}

var geminiToolingDefaults = buildGeminiToolingDefaults()

// buildGeminiToolingDefaults attaches channel-level web search pricing derived from Google documentation.
func buildGeminiToolingDefaults() adaptor.ChannelToolConfig {
	if len(geminiWebSearchModels) == 0 {
		return adaptor.ChannelToolConfig{}
	}
	return adaptor.ChannelToolConfig{
		Pricing: map[string]adaptor.ToolPricingConfig{
			"web_search": {UsdPerCall: geminiWebSearchUsdPerCall},
		},
	}
}

// GeminiToolingDefaults exposes the precomputed tooling defaults so callers
// can reuse them without rebuilding the configuration repeatedly.
func GeminiToolingDefaults() adaptor.ChannelToolConfig {
	return geminiToolingDefaults
}

const (
	// ModalityText is the text modality.
	ModalityText = "TEXT"
	// ModalityImage is the image modality.
	ModalityImage = "IMAGE"
)

var (
	geminiVersionPattern           = regexp.MustCompile(`^gemini-(\d+(?:\.\d+)?)(?:-|$)`)
	geminiResponseModalitiesCutoff = 2.5
)

// GetModelModalities returns the modalities of the model.
func GetModelModalities(model string) []string {
	normalized := strings.ToLower(model)
	if shouldOmitResponseModalities(normalized) {
		return nil
	}

	if strings.Contains(normalized, "-image") {
		return []string{ModalityText, ModalityImage}
	}

	return []string{ModalityText}
}

// shouldOmitResponseModalities reports whether the request should skip responseModalities.
func shouldOmitResponseModalities(model string) bool {
	if model == "aqa" || strings.HasPrefix(model, "gemma") || strings.HasPrefix(model, "text-embed") {
		return true
	}

	if GeminiVersionAtLeast(model, geminiResponseModalitiesCutoff) {
		return true
	}

	return false
}

// GeminiVersion returns the numeric Gemini version parsed from the model name along with a success flag.
// When the model name does not match the expected pattern, the flag is false.
func GeminiVersion(model string) (float64, bool) {
	if model == "" {
		return 0, false
	}

	matches := geminiVersionPattern.FindStringSubmatch(strings.ToLower(model))
	if len(matches) != 2 {
		return 0, false
	}

	version, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, false
	}

	return version, true
}

// GeminiVersionAtLeast reports whether the Gemini model name encodes a version greater than or equal to minVersion.
func GeminiVersionAtLeast(model string, minVersion float64) bool {
	version, ok := GeminiVersion(model)
	if !ok {
		return false
	}
	return version >= minVersion
}
