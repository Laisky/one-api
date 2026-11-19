package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// googleModels returns independently owned google defaults for togetherai.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://docs.together.ai/docs/serverless/models
func googleModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"google/flash-image-2.5": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.040894464,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.039),
			},
		},
		"google/flash-image-3.1": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.05,
				DefaultSize:      "1920x1080",
				MinImages:        1,
			},
		},
		"google/gemini-3-pro-image": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.134,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers:  gemini3ProImageSizeTiers(),
			},
		},
		"google/gemma-3n-E4B-it": {
			Ratio:                       nativeRate(0.06),
			CompletionRatio:             2,
			ContextLength:               32768,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp8",
			HuggingFaceID:               "google/gemma-3n-E4B-it",
			Description:                 "Google Gemma 3n E4B multimodal instruction model with vision support and 32K context.",
		},
		"google/gemma-4-31B-it": {
			Ratio:                       nativeRate(0.39),
			CompletionRatio:             0.97 / 0.39,
			ContextLength:               262144,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp8",
			HuggingFaceID:               "google/gemma-4-31b-it",
			Description:                 "Google Gemma 4 31B multimodal instruction model with 262K context.",
		},
		"google/imagen-4.0-fast": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.02097152,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.02),
			},
		},
		"google/imagen-4.0-preview": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04194304,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.04),
			},
		},
		"google/imagen-4.0-ultra": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.06291456,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				Megapixel:        perImageAreaTariff(0.06),
			},
		},
	}
}
