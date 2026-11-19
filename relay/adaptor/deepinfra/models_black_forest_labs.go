package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// flux2PixelsPerMegapixel is Black Forest Labs' megapixel: "One megapixel
// equals 1024 × 1024 pixels (1,048,576 pixels)" (https://bfl.ai/pricing).
const flux2PixelsPerMegapixel = 1024 * 1024

// flux2ReferenceFallbackMegapixels is billed for a reference image whose
// dimensions cannot be decoded. Neither DeepInfra nor BFL publishes an input
// size cap, so it is a conservative 4096 x 4096 (16 MP) reference.
const flux2ReferenceFallbackMegapixels = 16

// flux2MegapixelTariff returns the FLUX.2 [max]/[pro] tariff DeepInfra resells.
// DeepInfra's model pages state: "Output: The first megapixel is charged $X and
// then each subsequent MP will be charged $Y" and "Input: We charge $Z for each
// megapixel on the input (i.e. reference images for editing)". DeepInfra does
// not define a megapixel or its rounding; BFL, the upstream billed via
// usage_from_cost, does: "Resolution is rounded up to the next MP, separately
// for each reference image and the generated output." Width and height "Must
// be a multiple of 32", so a requested side is billed rounded up to 32 pixels.
// Parameters: first, additional, and reference are the USD prices of the first
// output megapixel, each further output megapixel, and each reference-image
// megapixel. Returns: a fresh tariff owned by the caller.
func flux2MegapixelTariff(first, additional, reference float64) *adaptor.ImageMegapixelPricing {
	return &adaptor.ImageMegapixelPricing{
		FirstMegapixelUsd:           first,
		AdditionalMegapixelUsd:      additional,
		ReferenceMegapixelUsd:       reference,
		PixelsPerMegapixel:          flux2PixelsPerMegapixel,
		Rounding:                    adaptor.ImageMegapixelRoundingCeil,
		OutputDimensionStep:         32,
		ReferenceFallbackMegapixels: flux2ReferenceFallbackMegapixels,
	}
}

// blackForestLabsModels returns independently owned black forest labs defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func blackForestLabsModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"black-forest-labs/FLUX-1.1-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
			Description:      "FLUX 1.1 Pro text-to-image model.",
		},
		"black-forest-labs/FLUX-2-klein-4b": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.014,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"512x512":   0.25,
					"768x768":   0.5625,
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "FLUX.2 Klein 4B text-to-image and image editing model.",
		},
		"black-forest-labs/FLUX-2-klein-9b": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.015,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"512x512":   0.25,
					"768x768":   0.5625,
				},
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "FLUX.2 Klein 9B text-to-image and image editing model.",
		},
		// https://deepinfra.com/black-forest-labs/FLUX-2-max: $0.07 first output
		// MP, $0.03 each further MP, $0.03 per reference-image MP. The flat
		// price is the 1 MP minimum shown in listings.
		"black-forest-labs/FLUX-2-max": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.07,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
				Megapixel:        flux2MegapixelTariff(0.07, 0.03, 0.03),
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "FLUX 2 Max image generation and editing model.",
		},
		// https://deepinfra.com/black-forest-labs/FLUX-2-pro: $0.03 first output
		// MP, $0.015 each further MP, $0.015 per reference-image MP. The page's
		// "$0.015 / image" headline is the additional-MP rate, not the price of
		// a 1 MP image, so the 1 MP minimum is $0.03.
		"black-forest-labs/FLUX-2-pro": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.03,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
				Megapixel:        flux2MegapixelTariff(0.03, 0.015, 0.015),
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "FLUX 2 Pro image generation and editing model.",
		},
	}
}
