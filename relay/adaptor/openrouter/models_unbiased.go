package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// unbiasedModels returns independently owned unbiased defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func unbiasedModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"unbiased/pareto": {
			Ratio:                       nativeRate(2.5),
			CompletionRatio:             3,
			CachedInputRatio:            nativeRate(0.25),
			ContextLength:               262144,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools"},
			SupportedSamplingParameters: []string{"max_tokens", "temperature", "tool_choice", "tools", "top_p"},
			Description:                 "unbiased/pareto on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"unbiased/pareto-26.10": {
			Ratio:                       nativeRate(0.8),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.03),
			ContextLength:               1048576,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools"},
			SupportedSamplingParameters: []string{"max_tokens", "temperature", "tool_choice", "tools", "top_p"},
			Description:                 "unbiased/pareto-26.10 on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"unbiased/pareto-26.10-preview": {
			Ratio:                       nativeRate(0.8),
			CompletionRatio:             (3.2 * 1.0) / 0.8,
			CachedInputRatio:            nativeRate(0.03),
			ContextLength:               1048576,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"max_tokens", "response_format", "temperature", "tool_choice", "tools", "top_p"},
			Description:                 "unbiased/pareto-26.10-preview on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"unbiased/pareto-26.9": {
			Ratio:                       nativeRate(2.5),
			CompletionRatio:             3,
			CachedInputRatio:            nativeRate(0.25),
			ContextLength:               262144,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"max_tokens", "response_format", "temperature", "tool_choice", "tools", "top_p"},
			Description:                 "unbiased/pareto-26.9 on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"~unbiased/pareto-latest": {
			Ratio:                       nativeRate(0.8),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.03),
			ContextLength:               1048576,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools"},
			SupportedSamplingParameters: []string{"max_tokens", "temperature", "tool_choice", "tools", "top_p"},
			Description:                 "~unbiased/pareto-latest on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
	}
}
