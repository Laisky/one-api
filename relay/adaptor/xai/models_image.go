package xai

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// imageModels returns the complete xAI image catalog, including retained historical IDs.
// It takes no parameters and keeps the effective definitions together by capability.
// Sources: https://docs.x.ai/developers/models and https://docs.x.ai/developers/pricing
// https://docs.x.ai/developers/models/grok-imagine-image-2.0
// https://docs.x.ai/developers/migration/imagine-image-quality-nov-2
func imageModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"grok-2-image": {
			Ratio:           0,
			CompletionRatio: 1.0,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.07,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        10,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
				},
			},
			ContextLength:    4000,
			InputModalities:  grokTextInputs,
			OutputModalities: []string{"image"},
			Description:      "Grok 2 Image — legacy image generation alias at $0.07/image (UNVERIFIED: no longer listed in docs.x.ai/developers/models).",
		},
		"grok-2-image-1212": {
			Ratio:           0,
			CompletionRatio: 1.0,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.07,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        10,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
				},
			},
			ContextLength:    4000,
			InputModalities:  grokTextInputs,
			OutputModalities: []string{"image"},
			Description:      "Grok 2 Image 1212 — legacy image generation snapshot at $0.07/image (UNVERIFIED: no longer listed in docs.x.ai/developers/models).",
		},
		"grok-imagine-image": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.02,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        10,
				SizeMultipliers:  map[string]float64{"1024x1024": 1.0, "2048x2048": 1.0},
			},
			ContextLength:    4000,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Grok Imagine Image — fast text+image-to-image generation at $0.02/image (alias: grok-imagine-image-2026-03-02).",
		}).Clone(),
		"grok-imagine-image-2.0": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "auto",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        10,
				QualitySizeMultipliers: map[string]map[string]float64{
					"low": {"1024x1024": 1.00, "2048x2048": 1.50},
					// Image generation resolves auto to low; image editing resolves auto to
					// medium through the controller's billing-only effective quality.
					"auto":   {"1024x1024": 1.00, "2048x2048": 1.50},
					"medium": {"1024x1024": 1.50, "2048x2048": 2.00},
				},
			},
			ContextLength:    4000,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Grok Imagine Image 2.0 supports text/image input and low, medium, or auto quality. Official output pricing spans 1K, 1.5K, and 2K tiers ($0.04-$0.08 per image); one-api currently exposes the upstream 1k and 2k resolution values.",
		}).Clone(),
		"grok-imagine-image-2026-03-02": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.02,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        10,
				SizeMultipliers:  map[string]float64{"1024x1024": 1.0, "2048x2048": 1.0},
			},
			ContextLength:    4000,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Grok Imagine Image snapshot (2026-03-02) — fast text+image-to-image generation at $0.02/image; alias of grok-imagine-image.",
		}).Clone(),
		"grok-imagine-image-pro": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.05,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        10,
				SizeMultipliers:  map[string]float64{"1024x1024": 1.0, "1408x1408": 1.2, "2048x2048": 1.4},
			},
			TimeWindows:      []adaptor.TimeWindow{legacyImagineQualityRedirectWindow()},
			ContextLength:    4000,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Legacy Imagine quality slug: $0.05/$0.06/$0.07 per image at 1K/1.5K/2K through November 1, 2026; from November 2 it redirects to Image 2.0 low quality at $0.04/$0.05/$0.06.",
		}).Clone(),
		"grok-imagine-image-quality": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.05,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        10,
				SizeMultipliers:  map[string]float64{"1024x1024": 1.0, "1408x1408": 1.2, "2048x2048": 1.4},
			},
			TimeWindows:      []adaptor.TimeWindow{legacyImagineQualityRedirectWindow()},
			ContextLength:    4000,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Legacy Imagine quality slug: $0.05/$0.06/$0.07 per image at 1K/1.5K/2K through November 1, 2026; from November 2 it redirects to Image 2.0 low quality at $0.04/$0.05/$0.06.",
		}).Clone(),
		"grok-imagine-image-quality-20260403": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.05,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        10,
				SizeMultipliers:  map[string]float64{"1024x1024": 1.0, "1408x1408": 1.2, "2048x2048": 1.4},
			},
			TimeWindows:      []adaptor.TimeWindow{legacyImagineQualityRedirectWindow()},
			ContextLength:    4000,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Legacy Imagine quality slug: $0.05/$0.06/$0.07 per image at 1K/1.5K/2K through November 1, 2026; from November 2 it redirects to Image 2.0 low quality at $0.04/$0.05/$0.06.",
		}).Clone(),
		"grok-imagine-image-quality-latest": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.05,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				PromptTokenLimit: 4000,
				MinImages:        1,
				MaxImages:        10,
				SizeMultipliers:  map[string]float64{"1024x1024": 1.0, "1408x1408": 1.2, "2048x2048": 1.4},
			},
			TimeWindows:      []adaptor.TimeWindow{legacyImagineQualityRedirectWindow()},
			ContextLength:    4000,
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Legacy Imagine quality slug: $0.05/$0.06/$0.07 per image at 1K/1.5K/2K through November 1, 2026; from November 2 it redirects to Image 2.0 low quality at $0.04/$0.05/$0.06.",
		}).Clone(),
	}
}

// legacyImagineQualityRedirectWindow switches legacy quality aliases to the
// Image 2.0 low-quality tariff at the documented November 2, 2026 migration.
// Parameters: none. Returns: an all-day UTC pricing overlay with no end date.
func legacyImagineQualityRedirectWindow() adaptor.TimeWindow {
	return adaptor.TimeWindow{
		Name:     "xai-imagine-quality-redirect",
		TimeZone: "UTC",
		DateFrom: "2026-11-02",
		Ranges: []adaptor.ClockRange{
			{Start: "00:00", End: "00:00"},
		},
		Overlay: adaptor.ModelConfig{
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1.00,
					"1408x1408": 1.25,
					"2048x2048": 1.50,
				},
			},
		},
	}
}
