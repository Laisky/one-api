package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// imageModelRatios captures pricing and metadata for OpenAI image generation
// and editing models. Per-image pricing remains in Image; the new modality and
// description fields advertise the model's request envelope to OpenRouter.
//
// ⚠️ should also update relay/billing/ratio/image.go when changing these values.
//
// Sources: https://platform.openai.com/docs/models/dall-e-3, /gpt-image-1.
//
// Policy: If a model is billed per image only, set Ratio=0 and configure
// Image.PricePerImageUsd. GPT Image models bill both prompt tokens and per-image
// output; keep Ratio in sync with prompt pricing while retaining
// Image.PricePerImageUsd for renders.
var imageModelRatios = map[string]adaptor.ModelConfig{
	"dall-e-2": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.016,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 1000,
			MinImages:        1,
			MaxImages:        10,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"256x256":   1,
					"512x512":   1.125,
					"1024x1024": 1.25,
				},
				"standard": {
					"256x256":   1,
					"512x512":   1.125,
					"1024x1024": 1.25,
				},
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DALL-E 2: legacy text-to-image diffusion model. (RETIRED 2026-05-12; use gpt-image-2)",
	},
	"dall-e-3": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.04,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        1,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1792": 2,
					"1792x1024": 2,
				},
				"standard": {
					"1024x1024": 1,
					"1024x1792": 2,
					"1792x1024": 2,
				},
				"hd": {
					"1024x1024": 2,
					"1024x1792": 3,
					"1792x1024": 3,
				},
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"image"},
		Description:      "DALL-E 3: advanced text-to-image model with HD quality option. (RETIRED 2026-05-12; use gpt-image-2)",
	},
	"gpt-image-1": {
		Ratio:            5.0 * ratio.MilliTokensUsd,
		CachedInputRatio: 1.25 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.011,
			DefaultSize:      "1024x1536",
			DefaultQuality:   "high",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        1,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1536": 16.0 / 11,
					"1536x1024": 16.0 / 11,
				},
				"low": {
					"1024x1024": 1,
					"1024x1536": 16.0 / 11,
					"1536x1024": 16.0 / 11,
				},
				"medium": {
					"1024x1024": 42.0 / 11,
					"1024x1536": 63.0 / 11,
					"1536x1024": 63.0 / 11,
				},
				"high": {
					"1024x1024": 167.0 / 11,
					"1024x1536": 250.0 / 11,
					"1536x1024": 250.0 / 11,
				},
				"auto": {
					"1024x1024": 167.0 / 11,
					"1024x1536": 250.0 / 11,
					"1536x1024": 250.0 / 11,
				},
			},
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "GPT Image 1: hybrid text+image input model that emits edited or generated images. (retires 2026-10-23; use gpt-image-2)",
	},
	"gpt-image-1-mini": {
		Ratio:            2.0 * ratio.MilliTokensUsd,
		CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.005,
			DefaultSize:      "1024x1536",
			DefaultQuality:   "high",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        1,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1536": 0.006 / 0.005,
					"1536x1024": 0.006 / 0.005,
				},
				"low": {
					"1024x1024": 1,
					"1024x1536": 0.006 / 0.005,
					"1536x1024": 0.006 / 0.005,
				},
				"medium": {
					"1024x1024": 0.011 / 0.005,
					"1024x1536": 0.015 / 0.005,
					"1536x1024": 0.015 / 0.005,
				},
				"high": {
					"1024x1024": 0.036 / 0.005,
					"1024x1536": 0.052 / 0.005,
					"1536x1024": 0.052 / 0.005,
				},
				"auto": {
					"1024x1024": 0.036 / 0.005,
					"1024x1536": 0.052 / 0.005,
					"1536x1024": 0.052 / 0.005,
				},
			},
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "GPT Image 1 mini: lower-cost hybrid text+image generation model. (retires 2026-12-01; use gpt-image-2)",
	},
	"chatgpt-image-latest": {
		Ratio:            5.0 * ratio.MilliTokensUsd,
		CachedInputRatio: 1.25 * ratio.MilliTokensUsd,
		CompletionRatio:  2.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.009,
			DefaultSize:      "1024x1536",
			DefaultQuality:   "high",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        1,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1536": 13.0 / 9.0,
					"1536x1024": 13.0 / 9.0,
				},
				"low": {
					"1024x1024": 1,
					"1024x1536": 13.0 / 9.0,
					"1536x1024": 13.0 / 9.0,
				},
				"medium": {
					"1024x1024": 34.0 / 9.0,
					"1024x1536": 50.0 / 9.0,
					"1536x1024": 50.0 / 9.0,
				},
				"high": {
					"1024x1024": 133.0 / 9.0,
					"1024x1536": 200.0 / 9.0,
					"1536x1024": 200.0 / 9.0,
				},
				"auto": {
					"1024x1024": 133.0 / 9.0,
					"1024x1536": 200.0 / 9.0,
					"1536x1024": 200.0 / 9.0,
				},
			},
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "ChatGPT image latest: rolling alias for the consumer ChatGPT image model. (retires 2026-12-01; use gpt-image-2)",
	},
	"gpt-image-1.5": {
		Ratio:            5.0 * ratio.MilliTokensUsd,
		CachedInputRatio: 1.25 * ratio.MilliTokensUsd,
		CompletionRatio:  2.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.009,
			DefaultSize:      "1024x1536",
			DefaultQuality:   "high",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        1,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1536": 13.0 / 9.0,
					"1536x1024": 13.0 / 9.0,
				},
				"low": {
					"1024x1024": 1,
					"1024x1536": 13.0 / 9.0,
					"1536x1024": 13.0 / 9.0,
				},
				"medium": {
					"1024x1024": 34.0 / 9.0,
					"1024x1536": 50.0 / 9.0,
					"1536x1024": 50.0 / 9.0,
				},
				"high": {
					"1024x1024": 133.0 / 9.0,
					"1024x1536": 200.0 / 9.0,
					"1536x1024": 200.0 / 9.0,
				},
				"auto": {
					"1024x1024": 133.0 / 9.0,
					"1024x1536": 200.0 / 9.0,
					"1536x1024": 200.0 / 9.0,
				},
			},
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "GPT Image 1.5: hybrid text+image generation/editing model. (retires 2026-12-01; use gpt-image-2)",
	},
	"gpt-image-1.5-2025-12-16": {
		Ratio:            5.0 * ratio.MilliTokensUsd,
		CachedInputRatio: 1.25 * ratio.MilliTokensUsd,
		CompletionRatio:  2.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.009,
			DefaultSize:      "1024x1536",
			DefaultQuality:   "high",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        1,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1536": 13.0 / 9.0,
					"1536x1024": 13.0 / 9.0,
				},
				"low": {
					"1024x1024": 1,
					"1024x1536": 13.0 / 9.0,
					"1536x1024": 13.0 / 9.0,
				},
				"medium": {
					"1024x1024": 34.0 / 9.0,
					"1024x1536": 50.0 / 9.0,
					"1536x1024": 50.0 / 9.0,
				},
				"high": {
					"1024x1024": 133.0 / 9.0,
					"1024x1536": 200.0 / 9.0,
					"1536x1024": 200.0 / 9.0,
				},
				"auto": {
					"1024x1024": 133.0 / 9.0,
					"1024x1536": 200.0 / 9.0,
					"1536x1024": 200.0 / 9.0,
				},
			},
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "GPT Image 1.5 snapshot from 2025-12-16. (retires 2026-12-01; use gpt-image-2)",
	},
	"gpt-image-2": {
		Ratio:            5.0 * ratio.MilliTokensUsd,
		CachedInputRatio: 1.25 * ratio.MilliTokensUsd,
		CompletionRatio:  2.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.006,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "auto",
			PromptTokenLimit: 32000,
			MinImages:        1,
			MaxImages:        10,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1536": 5.0 / 6.0,
					"1536x1024": 5.0 / 6.0,
				},
				"low": {
					"1024x1024": 1,
					"1024x1536": 5.0 / 6.0,
					"1536x1024": 5.0 / 6.0,
				},
				"medium": {
					"1024x1024": 53.0 / 6.0,
					"1024x1536": 41.0 / 6.0,
					"1536x1024": 41.0 / 6.0,
				},
				"high": {
					"1024x1024": 211.0 / 6.0,
					"1024x1536": 165.0 / 6.0,
					"1536x1024": 165.0 / 6.0,
				},
				"auto": {
					"1024x1024": 211.0 / 6.0,
					"1024x1536": 165.0 / 6.0,
					"1536x1024": 165.0 / 6.0,
				},
			},
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "GPT Image 2: next-gen multimodal image generation/editing model.",
	},
	"gpt-image-2-2026-04-21": {
		Ratio:            5.0 * ratio.MilliTokensUsd,
		CachedInputRatio: 1.25 * ratio.MilliTokensUsd,
		CompletionRatio:  2.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.006,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "auto",
			PromptTokenLimit: 32000,
			MinImages:        1,
			MaxImages:        10,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1536": 5.0 / 6.0,
					"1536x1024": 5.0 / 6.0,
				},
				"low": {
					"1024x1024": 1,
					"1024x1536": 5.0 / 6.0,
					"1536x1024": 5.0 / 6.0,
				},
				"medium": {
					"1024x1024": 53.0 / 6.0,
					"1024x1536": 41.0 / 6.0,
					"1536x1024": 41.0 / 6.0,
				},
				"high": {
					"1024x1024": 211.0 / 6.0,
					"1024x1536": 165.0 / 6.0,
					"1536x1024": 165.0 / 6.0,
				},
				"auto": {
					"1024x1024": 211.0 / 6.0,
					"1024x1536": 165.0 / 6.0,
					"1536x1024": 165.0 / 6.0,
				},
			},
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      "GPT Image 2 snapshot from 2026-04-21.",
	},
	"gpt-image-2.5-sunburst": newGPTImageTokenConfig(
		"GPT Image 2.5 Sunburst: high-quality image generation and precision editing; supports low, medium, high, xhigh, max, and auto quality."),
	"gpt-image-2.5-sunburst-2026-09-08": newGPTImageTokenConfig(
		"GPT Image 2.5 Sunburst snapshot (2026-09-08): high-quality image generation and precision editing; supports low, medium, high, xhigh, max, and auto quality."),
	"gpt-image-2.5-flare": newGPTImageTokenConfig(
		"GPT Image 2.5 Flare: fast, high-quality everyday image generation and editing; supports low, medium, high, xhigh, max, and auto quality."),
	"gpt-image-2.5-flare-2026-09-08": newGPTImageTokenConfig(
		"GPT Image 2.5 Flare snapshot (2026-09-08): fast, high-quality everyday image generation and editing; supports low, medium, high, xhigh, max, and auto quality."),
	"gpt-image-2.6-sunburst": newGPTImageTokenConfig(
		"GPT Image 2.6 Sunburst: image generation and editing for detailed visual work; billed from text/image input and image output tokens."),
	"gpt-image-2.6-flare": newGPTImageTokenConfig(
		"GPT Image 2.6 Flare: image generation and editing for fast iteration; billed from text/image input and image output tokens."),
}

// GPT Image 2.6 sources verified 2026-11-16:
// https://developers.openai.com/api/docs/models/gpt-image-2.6-sunburst
// https://developers.openai.com/api/docs/models/gpt-image-2.6-flare
// https://developers.openai.com/api/docs/pricing
//
// newGPTImageTokenConfig creates independent metadata for a GPT Image 2.5 or 2.6 model.
// Parameters: description identifies the alias or snapshot. Returns: token-only
// pricing and Image API defaults without undocumented context or output limits.
func newGPTImageTokenConfig(description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:            5 * ratio.MilliTokensUsd,
		CachedInputRatio: 1.25 * ratio.MilliTokensUsd,
		CompletionRatio:  30.0 / 5.0, // Image output, not text output.
		Image: &adaptor.ImagePricingConfig{
			// Leave PricePerImageUsd and tier multipliers unset: the controller
			// reconciles actual image-token usage instead of a fixed render fee.
			PromptRatio:      8.0 / 5.0,
			DefaultSize:      "auto",
			DefaultQuality:   "auto",
			PromptTokenLimit: 32000, // Image API prompt-length limit; not context tokens.
			MinImages:        1,
			MaxImages:        10,
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      description,
	}
}
