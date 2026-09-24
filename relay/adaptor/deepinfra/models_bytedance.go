package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// bytedanceModels returns the bytedance model defaults for deepinfra.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func bytedanceModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"ByteDance/Seed-1.8": {
			Ratio:                       nativeRate(0.25),
			CompletionRatio:             8,
			CachedInputRatio:            nativeRate(0.05),
			ContextLength:               256000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "ByteDance/Seed-1.8 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ByteDance/Seed-2.0-code": {
			Ratio:                       nativeRate(0.5),
			CompletionRatio:             5.999999999999999,
			CachedInputRatio:            nativeRate(0.1),
			ContextLength:               256000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "ByteDance/Seed-2.0-code on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ByteDance/Seed-2.0-mini": {
			Ratio:                       nativeRate(0.1),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.020000000000000004),
			ContextLength:               256000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "ByteDance/Seed-2.0-mini on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ByteDance/Seed-2.0-pro": {
			Ratio:                       nativeRate(0.5),
			CompletionRatio:             5.999999999999999,
			CachedInputRatio:            nativeRate(0.1),
			ContextLength:               256000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "ByteDance/Seed-2.0-pro on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ByteDance/Seedream-4": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Seedream 4 multimodal image generation and editing model.",
		},
		"ByteDance/Seedream-4.5": {
			Ratio:           0,
			CompletionRatio: 1,
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.04,
				DefaultSize:      "1024x1024",
				DefaultQuality:   "standard",
				MinImages:        1,
				MaxImages:        4,
			},
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"image"},
			Description:      "Seedream 4.5 multimodal image generation and editing model.",
		},
	}
}
