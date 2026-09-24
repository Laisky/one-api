package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// googleModels returns the google model defaults for togetherai.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func googleModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"google/flash-image-2.5": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.040894464,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
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
				DefaultSize:      "1920x1080",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1920x1080": 1,
					"2048x2048": 1,
					"3840x2160": 1.791044776119403,
				},
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
			CompletionRatio:             2.4871794871794872,
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
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
		"google/imagen-4.0-preview": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04194304,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
		"google/imagen-4.0-ultra": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.06291456,
				DefaultSize:      "1024x1024",
				MinImages:        1,
				SizeMultipliers: map[string]float64{
					"1024x1024": 1,
					"1024x1536": 1.5,
					"1536x1024": 1.5,
					"1920x1080": 1.9775390625,
					"2048x2048": 4,
					"3840x2160": 7.91015625,
					"4096x4096": 16,
					"512x512":   0.25,
				},
			},
		},
	}
}
