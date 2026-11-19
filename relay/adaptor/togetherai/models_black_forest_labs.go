package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// blackForestLabsModels returns independently owned black forest labs defaults for togetherai.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://docs.together.ai/docs/serverless/models
//
// Together's Images API has no size field and renders the width and height it
// receives, so every tariff here is priced on the forwarded dimensions.
// FLUX.2 [max], FLUX.1 Kontext [pro]/[max] and FLUX1.1 [pro] are listed per
// megapixel (MP = width x height / 1,000,000) with published default steps in
// ImageBillingRuleFor; FLUX.2 [dev]/[flex]/[pro] are listed per image at their
// lowest resolution ("$0.0154+ (varies)"). PricePerImageUsd keeps the
// 1024x1024 price shown in listings; the megapixel tariff replaces it.
func blackForestLabsModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"black-forest-labs/FLUX.1-kontext-max": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.08388608,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perMegapixelImageTariff(0.08),
			},
		},
		"black-forest-labs/FLUX.1-kontext-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04194304,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perMegapixelImageTariff(0.04),
			},
		},
		"black-forest-labs/FLUX.1-krea-dev": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0262144,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perMegapixelImageTariff(0.025),
			},
			Description: "FLUX.1 Krea Dev image model. (RETIRED 2026-05-27)",
		},
		"black-forest-labs/FLUX.1-schnell": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0028311552,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perMegapixelImageTariff(0.0027),
			},
		},
		"black-forest-labs/FLUX.1.1-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04194304,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perMegapixelImageTariff(0.04),
			},
		},
		"black-forest-labs/FLUX.2-dev": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.0161480704,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.0154),
			},
		},
		"black-forest-labs/FLUX.2-flex": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.03145728,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.03),
			},
		},
		"black-forest-labs/FLUX.2-max": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.07340032,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perMegapixelImageTariff(0.07),
			},
		},
		"black-forest-labs/FLUX.2-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.03145728,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.03),
			},
		},
	}
}
