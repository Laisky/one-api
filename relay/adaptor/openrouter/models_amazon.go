package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// amazonModels returns independently owned amazon defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func amazonModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"amazon/nova-2-lite-v1": {
			Ratio:                       nativeRate(0.3),
			CompletionRatio:             8.333333333333334,
			ContextLength:               1000000,
			MaxOutputTokens:             65535,
			InputModalities:             []string{"text", "image", "video", "file"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"include_reasoning", "max_tokens", "reasoning", "stop", "temperature", "tool_choice", "tools", "top_k", "top_p"},
			Description:                 "amazon/nova-2-lite-v1 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"amazon/nova-lite-v1": {
			Ratio:                       nativeRate(0.06),
			CompletionRatio:             4,
			ContextLength:               300000,
			MaxOutputTokens:             5120,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools"},
			SupportedSamplingParameters: []string{"max_tokens", "stop", "temperature", "tools", "top_k", "top_p"},
			Description:                 "amazon/nova-lite-v1 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"amazon/nova-micro-v1": {
			Ratio:                       nativeRate(0.035),
			CompletionRatio:             4,
			ContextLength:               128000,
			MaxOutputTokens:             5120,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools"},
			SupportedSamplingParameters: []string{"max_tokens", "stop", "temperature", "tools", "top_k", "top_p"},
			Description:                 "amazon/nova-micro-v1 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"amazon/nova-premier-v1": {
			Ratio:                       nativeRate(2.5),
			CompletionRatio:             5,
			CachedInputRatio:            nativeRate(0.625),
			ContextLength:               1000000,
			MaxOutputTokens:             32000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools"},
			SupportedSamplingParameters: []string{"max_tokens", "stop", "temperature", "tools", "top_k", "top_p"},
			Description:                 "amazon/nova-premier-v1 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"amazon/nova-pro-v1": {
			Ratio:                       nativeRate(0.7999999999999999),
			CompletionRatio:             4,
			ContextLength:               300000,
			MaxOutputTokens:             5120,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools"},
			SupportedSamplingParameters: []string{"max_tokens", "stop", "temperature", "tools", "top_k", "top_p"},
			Description:                 "amazon/nova-pro-v1 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
