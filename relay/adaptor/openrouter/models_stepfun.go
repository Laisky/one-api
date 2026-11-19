package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// stepfunModels returns independently owned stepfun defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func stepfunModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"stepfun/step-3.5-flash": {
			Ratio:                       nativeRate(0.1),
			CompletionRatio:             3,
			ContextLength:               262144,
			MaxOutputTokens:             65536,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "max_tokens", "reasoning", "temperature", "tool_choice", "tools", "top_k", "top_p"},
			HuggingFaceID:               "stepfun-ai/Step-3.5-Flash",
			Description:                 "stepfun/step-3.5-flash on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"stepfun/step-3.7-flash": {
			Ratio:                       nativeRate(0.2),
			CompletionRatio:             5.75,
			CachedInputRatio:            nativeRate(0.04),
			ContextLength:               262144,
			MaxOutputTokens:             230400,
			InputModalities:             []string{"text", "image", "video"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logprobs", "max_tokens", "presence_penalty", "reasoning", "reasoning_effort", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_logprobs", "top_p"},
			SupportedReasoningEfforts:   []string{"high", "medium", "low"},
			DefaultReasoningEffort:      "medium",
			HuggingFaceID:               "stepfun-ai/Step-3.7-Flash",
			Description:                 "stepfun/step-3.7-flash on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"stepfun/step-5-preview": {
			Ratio:                       nativeRate(1),
			CompletionRatio:             (2.7 * 1.0) / 1,
			CachedInputRatio:            nativeRate(0.1),
			ContextLength:               1000000,
			MaxOutputTokens:             64000,
			InputModalities:             []string{"text", "image", "video"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logprobs", "max_tokens", "min_p", "presence_penalty", "reasoning", "reasoning_effort", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_logprobs", "top_p"},
			SupportedReasoningEfforts:   []string{"high", "medium", "low"},
			DefaultReasoningEffort:      "medium",
			Description:                 "stepfun/step-5-preview on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
			HuggingFaceID:               "stepfun-ai/Step-5-Preview",
		},
	}
}
