package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// openaiModels returns independently owned openai defaults for togetherai.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://docs.together.ai/docs/serverless/models
func openaiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"openai/gpt-image-1.5": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.034,
				DefaultSize:      "1024x1024",
				MinImages:        1,
			},
		},
		"openai/gpt-image-2": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.053,
				DefaultSize:      "1024x1024",
				MinImages:        1,
			},
		},
		"openai/gpt-oss-120b": {
			Ratio:                       nativeRate(0.15),
			CompletionRatio:             (0.6 * 1.0) / 0.15,
			ContextLength:               131072,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			SupportedReasoningEfforts:   []string{"low", "medium", "high"},
			DefaultReasoningEffort:      "medium",
			Quantization:                "fp4",
			HuggingFaceID:               "openai/gpt-oss-120b",
			Description:                 "openai/gpt-oss-120b on Together's public serverless API; verified 2026-10-12. Token tariffs are USD per million. Omitted source fields retain prior metadata rather than becoming invented limits or free prices. Host quantization: MXFP4; represented by the generic fp4 metadata label.",
		},
		"openai/gpt-oss-20b": {
			Ratio:                       nativeRate(0.05),
			CompletionRatio:             4,
			ContextLength:               128000,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			SupportedReasoningEfforts:   []string{"low", "medium", "high"},
			DefaultReasoningEffort:      "medium",
			Quantization:                "fp4",
			HuggingFaceID:               "openai/gpt-oss-20b",
			Description:                 "OpenAI gpt-oss-20b compact dense open-weight model with 128K context.",
		},
		"openai/whisper-large-v3": {
			Ratio:           0,
			CompletionRatio: 1,
			Audio: &adaptor.AudioPricingConfig{
				UsdPerSecond: 0.000025,
			},
		},
	}
}
