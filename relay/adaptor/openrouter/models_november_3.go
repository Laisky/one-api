package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// novemberChatModels3 returns independent host-specific chat defaults checked 2026-11-16.
// It takes no parameters and returns static Go configurations; account limits remain provider-controlled.
// Source: https://openrouter.ai/api/v1/models
func novemberChatModels3() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"scaledown/summarize": {
			Ratio:                       nativeRate(0.04),
			CompletionRatio:             0,
			ContextLength:               1000000,
			MaxOutputTokens:             4000,
			InputModalities:             []string{"text", "image", "file"},
			OutputModalities:            []string{"text"},
			SupportedSamplingParameters: []string{"max_tokens"},
			Description:                 "scaledown/summarize on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"scaledown/extract": {
			Ratio:                       nativeRate(0.04),
			CompletionRatio:             0,
			ContextLength:               1000000,
			MaxOutputTokens:             900000,
			InputModalities:             []string{"text", "image", "file"},
			OutputModalities:            []string{"text"},
			SupportedSamplingParameters: []string{"max_tokens"},
			Description:                 "scaledown/extract on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"inclusionai/ling-3.0-tiny": {
			Ratio:                       nativeRate(0.015),
			CompletionRatio:             3,
			CachedInputRatio:            nativeRate(0.003),
			ContextLength:               262144,
			MaxOutputTokens:             250000,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logprobs", "max_tokens", "presence_penalty", "reasoning", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_logprobs", "top_p"},
			HuggingFaceID:               "inclusionAI/Ling-3.0-tiny",
			Description:                 "inclusionai/ling-3.0-tiny on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
	}
}
