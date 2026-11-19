package geminiOpenaiCompatible

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
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

	gemini31FlashImage512Price = 0.045
	gemini31FlashImage1KPrice  = 0.067
	gemini31FlashImage2KPrice  = 0.101
	gemini31FlashImage4KPrice  = 0.151 // 4K (2520 tokens) = $0.151/image per Google pricing footnote

	geminiEmbedding001TextPrice              = 0.15
	geminiEmbedding2PreviewTextPrice         = 0.20
	geminiEmbedding2PreviewImagePrice        = 0.45
	geminiEmbedding2PreviewAudioPrice        = 6.50
	geminiEmbedding2PreviewVideoPrice        = 12.00
	geminiEmbedding2PreviewUsdPerImage       = 0.00012
	geminiEmbedding2PreviewUsdPerAudioSecond = 0.00016
	geminiEmbedding2PreviewUsdPerVideoFrame  = 0.00079
)

// geminiEmbedding2PreviewConfig returns the multimodal embedding pricing metadata Google publishes for Gemini embedding preview.
func geminiEmbedding2PreviewConfig() *adaptor.EmbeddingPricingConfig {
	return &adaptor.EmbeddingPricingConfig{
		TextTokenRatio:    geminiEmbedding2PreviewTextPrice * ratio.MilliTokensUsd,
		ImageTokenRatio:   geminiEmbedding2PreviewImagePrice * ratio.MilliTokensUsd,
		AudioTokenRatio:   geminiEmbedding2PreviewAudioPrice * ratio.MilliTokensUsd,
		VideoTokenRatio:   geminiEmbedding2PreviewVideoPrice * ratio.MilliTokensUsd,
		UsdPerImage:       geminiEmbedding2PreviewUsdPerImage,
		UsdPerAudioSecond: geminiEmbedding2PreviewUsdPerAudioSecond,
		UsdPerVideoFrame:  geminiEmbedding2PreviewUsdPerVideoFrame,
	}
}

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

// gemini31FlashImageConfig encodes the image-size tiered pricing Google published for Gemini 3.1 Flash Image Preview.
// 512/1K/2K/4K outputs are billed at $0.045/$0.067/$0.101/$0.151 per image.
func gemini31FlashImageConfig() *adaptor.ImagePricingConfig {
	return &adaptor.ImagePricingConfig{
		PricePerImageUsd: gemini31FlashImage1KPrice,
		DefaultSize:      "1024x1024",
		DefaultQuality:   "standard",
		MinImages:        1,
		SizeMultipliers: map[string]float64{
			"512x512":   gemini31FlashImage512Price / gemini31FlashImage1KPrice,
			"1024x1024": 1,
			"2048x2048": gemini31FlashImage2KPrice / gemini31FlashImage1KPrice,
			"4096x4096": gemini31FlashImage4KPrice / gemini31FlashImage1KPrice,
		},
	}
}

var (
	gemini25ProPricing = adaptor.ModelConfig{
		Ratio:            1.25 * ratio.MilliTokensUsd,
		CompletionRatio:  10.0 / 1.25,
		CachedInputRatio: 0.125 * ratio.MilliTokensUsd,
		Tiers: []adaptor.ModelRatioTier{
			{
				Ratio:               2.5 * ratio.MilliTokensUsd,
				CompletionRatio:     15.0 / 2.5,
				CachedInputRatio:    0.25 * ratio.MilliTokensUsd,
				InputTokenThreshold: 200001,
			},
		},
	}
	gemini25FlashPricing = adaptor.ModelConfig{
		Ratio:            0.30 * ratio.MilliTokensUsd,
		CompletionRatio:  2.50 / 0.30,
		CachedInputRatio: 0.03 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1.00 / 0.30,
			CompletionRatio: 0.30,
		},
	}
	gemini25FlashLitePricing = adaptor.ModelConfig{
		Ratio:            0.10 * ratio.MilliTokensUsd,
		CompletionRatio:  0.40 / 0.10,
		CachedInputRatio: 0.01 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     0.30 / 0.10,
			CompletionRatio: 0.10 / 0.30,
		},
	}
	gemini3FlashPricing = adaptor.ModelConfig{
		Ratio:            0.50 * ratio.MilliTokensUsd,
		CompletionRatio:  3.00 / 0.50,
		CachedInputRatio: 0.05 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1.00 / 0.50,
			CompletionRatio: 3.00 / 1.00,
		},
	}
	// gemini35FlashPricing reflects the Gemini 3.5 Flash GA pricing announced 2026-05-19.
	// Source: https://ai.google.dev/gemini-api/docs/pricing — $1.50 input / $9.00 output,
	// cached $0.15. Google publishes a single unified input rate across text/image/audio/video.
	gemini35FlashPricing = adaptor.ModelConfig{
		Ratio:            1.50 * ratio.MilliTokensUsd,
		CompletionRatio:  9.00 / 1.50,
		CachedInputRatio: 0.15 * ratio.MilliTokensUsd,
	}
	// gemini36FlashPricing reflects Google Cloud Agent Platform standard pricing:
	// $1.50 input / $7.50 output / $0.15 cached, with unified text/image/video/audio input.
	gemini36FlashPricing = adaptor.ModelConfig{
		Ratio:            1.50 * ratio.MilliTokensUsd,
		CompletionRatio:  7.50 / 1.50,
		CachedInputRatio: 0.15 * ratio.MilliTokensUsd,
	}
	// gemini35FlashLitePricing reflects Google Cloud Agent Platform standard pricing:
	// $0.30 input / $2.50 output / $0.03 cached, with unified text/image/video/audio input.
	gemini35FlashLitePricing = adaptor.ModelConfig{
		Ratio:            0.30 * ratio.MilliTokensUsd,
		CompletionRatio:  2.50 / 0.30,
		CachedInputRatio: 0.03 * ratio.MilliTokensUsd,
	}
	gemini31FlashLivePreviewPricing = adaptor.ModelConfig{
		Ratio:           0.75 * ratio.MilliTokensUsd,
		CompletionRatio: 4.50 / 0.75,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     3.00 / 0.75,
			CompletionRatio: 12.00 / 4.50,
		},
	}
	// gemini31FlashLitePricing applies to both the preview snapshot and the GA tier.
	// Source: https://ai.google.dev/gemini-api/docs/pricing — $0.25 input / $1.50 output,
	// cached $0.025, audio input $0.50.
	gemini31FlashLitePricing = adaptor.ModelConfig{
		Ratio:            0.25 * ratio.MilliTokensUsd,
		CompletionRatio:  1.50 / 0.25,
		CachedInputRatio: 0.025 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     0.50 / 0.25,
			CompletionRatio: 0.25 / 0.50,
		},
	}
	// gemini25FlashNativeAudioPricing covers all native-audio preview snapshots.
	// Source: https://ai.google.dev/gemini-api/docs/pricing — text input $0.50, text output $2.00,
	// audio in/out $3.00 / $12.00.
	gemini25FlashNativeAudioPricing = adaptor.ModelConfig{
		Ratio:           0.50 * ratio.MilliTokensUsd,
		CompletionRatio: 2.0 / 0.50,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     3.0 / 0.50,
			CompletionRatio: 12.0 / 2.0,
		},
	}
	// geminiRoboticsER15Pricing is documented as a Flash-tier billing surface in Google's pricing table.
	geminiRoboticsER15Pricing = adaptor.ModelConfig{
		Ratio:           0.30 * ratio.MilliTokensUsd,
		CompletionRatio: 2.5 / 0.30,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     1.00 / 0.30,
			CompletionRatio: 0.30,
		},
	}
	// geminiRoboticsER16Pricing reflects Google's new preview tier ($1.00 input text/image/video,
	// $2.00 audio input, $5.00 output) per https://ai.google.dev/gemini-api/docs/pricing.
	geminiRoboticsER16Pricing = adaptor.ModelConfig{
		Ratio:           1.00 * ratio.MilliTokensUsd,
		CompletionRatio: 5.00 / 1.00,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:     2.00 / 1.00,
			CompletionRatio: 5.00 / 2.00,
		},
	}
)

const (
	// geminiWebSearchUsdPerCall is the grounded web search price for Gemini 2.5 models: $35/1K queries.
	// Sources verified 2026-06-13:
	//   - https://ai.google.dev/gemini-api/docs/models
	//   - https://ai.google.dev/gemini-api/docs/pricing
	geminiWebSearchUsdPerCall = 35.0 / 1000.0
	// gemini3xWebSearchUsdPerCall is the grounded web search price for Gemini 3.x models: $14/1K queries.
	gemini3xWebSearchUsdPerCall = 14.0 / 1000.0
)

// geminiWebSearchModels enumerates Gemini models with grounded web search pricing in Google documentation.
// Sources:
//   - https://ai.google.dev/gemini-api/docs/pricing
//   - https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing
var geminiWebSearchModels = map[string]struct{}{
	"gemini-3.1-pro-preview":                  {},
	"gemini-3.1-pro-preview-customtools":      {},
	"gemini-3.1-flash-image-preview":          {},
	"gemini-3.1-flash-lite":                   {},
	"gemini-3.1-flash-lite-preview":           {},
	"gemini-3-pro-preview":                    {},
	"gemini-3-flash-preview":                  {},
	"gemini-3.5-flash":                        {},
	"gemini-3.6-flash":                        {},
	"gemini-3.5-flash-lite":                   {},
	"gemini-2.5-pro":                          {},
	"gemini-2.5-pro-preview":                  {},
	"gemini-2.5-computer-use-preview":         {},
	"gemini-2.5-computer-use-preview-10-2025": {},
	"gemini-2.5-flash":                        {},
	"gemini-2.5-flash-preview":                {},
	"gemini-2.5-flash-lite":                   {},
	"gemini-2.5-flash-lite-preview":           {},
	"gemini-2.0-flash":                        {},
	"gemini-2.0-flash-image":                  {},
	"gemini-2.0-flash-lite":                   {},
	"gemini-robotics-er-1.5-preview":          {},
	"gemini-robotics-er-1.6-preview":          {},

	"gemini-3.8-flash":                       {},
	"gemini-3.7-flash":                       {},
	"gemini-robotics-er-2-preview":           {},
	"gemini-robotics-er-2-streaming-preview": {},
	"gemini-3.8-live":                        {},
	"gemini-3.8-live-extended-thinking":      {},
	"gemini-4-argon":                         {},
	"gemini-4-carbon":                        {},
	"gemini-4-hydrogen":                      {},
}

var (
	geminiToolingDefaults   = buildGeminiToolingDefaults(geminiWebSearchUsdPerCall)
	gemini3xToolingDefaults = buildGeminiToolingDefaults(gemini3xWebSearchUsdPerCall)
)

// buildGeminiToolingDefaults attaches channel-level web search pricing derived from Google documentation.
func buildGeminiToolingDefaults(webSearchUsdPerCall float64) adaptor.ChannelToolConfig {
	if len(geminiWebSearchModels) == 0 {
		return adaptor.ChannelToolConfig{}
	}
	return adaptor.ChannelToolConfig{
		Pricing: map[string]adaptor.ToolPricingConfig{
			"web_search": {UsdPerCall: webSearchUsdPerCall},
		},
	}
}

// GeminiToolingDefaults exposes the precomputed tooling defaults (Gemini 2.5 pricing) so callers
// can reuse them without rebuilding the configuration repeatedly.
func GeminiToolingDefaults() adaptor.ChannelToolConfig {
	return geminiToolingDefaults
}

// GeminiToolingDefaultsForModel returns the tooling defaults for the given model,
// selecting $14/1K queries for Gemini 3.x models and $35/1K queries for 2.5 and earlier.
func GeminiToolingDefaultsForModel(model string) adaptor.ChannelToolConfig {
	// The upstream offers search, but this gateway cannot send native search
	// tools for Nano Banana yet. Expose no pricing that could imply admission.
	if IsGeminiNanoBanana21Model(model) {
		return adaptor.ChannelToolConfig{}
	}
	if GeminiVersionAtLeast(model, 3.0) {
		return gemini3xToolingDefaults
	}
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

// IsGeminiNanoBanana21Model matches this named upstream ID regardless of case.
// The same predicate is shared by native tool admission, image discovery, and
// Vertex global routing so a channel mapping cannot bypass model restrictions.
func IsGeminiNanoBanana21Model(model string) bool {
	return strings.EqualFold(model, GeminiNanoBanana21Model)
}

// GetModelModalities returns the modalities of the model.
func GetModelModalities(model string) []string {
	// The named Nano Banana model does not match the numeric Gemini version rule;
	// omitting IMAGE would turn image requests into text-only generation.
	if IsGeminiNanoBanana21Model(model) {
		return []string{ModalityText, ModalityImage}
	}
	normalized := strings.ToLower(model)
	if shouldOmitResponseModalities(normalized) {
		return nil
	}

	if IsGeminiImageGenerationModel(normalized) {
		return []string{ModalityText, ModalityImage}
	}

	return []string{ModalityText}
}

// IsGeminiImageGenerationModel checks both numeric Gemini image IDs and the
// named Nano Banana family. Parameters: model is the upstream ID. Returns: whether
// the model generates images and needs image-safe generation configuration.
func IsGeminiImageGenerationModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "-image") || IsGeminiNanoBanana21Model(model)
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
