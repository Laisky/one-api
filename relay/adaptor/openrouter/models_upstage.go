package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// upstageModels returns independently owned upstage defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func upstageModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"upstage/solar-mini4": {
			Ratio:                       nativeRate(0.05),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.005),
			ContextLength:               524288,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "max_tokens", "parallel_tool_calls", "presence_penalty", "reasoning", "reasoning_effort", "response_format", "structured_outputs", "temperature", "tool_choice", "tools", "top_p"},
			SupportedReasoningEfforts:   []string{"max", "xhigh", "high", "medium", "low", "minimal", "none"},
			DefaultReasoningEffort:      "medium",
			Description:                 "upstage/solar-mini4 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"upstage/solar-pro-3": {
			Ratio:                       nativeRate(0.15),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.015),
			ContextLength:               131072,
			MaxOutputTokens:             117964,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "max_tokens", "parallel_tool_calls", "presence_penalty", "reasoning", "reasoning_effort", "response_format", "structured_outputs", "temperature", "tool_choice", "tools", "top_p"},
			SupportedReasoningEfforts:   []string{"high", "medium", "low", "minimal", "none"},
			DefaultReasoningEffort:      "medium",
			Description:                 "upstage/solar-pro-3 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"upstage/solar-pro4": {
			Ratio:                       nativeRate(0.15),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.03),
			ContextLength:               524288,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logprobs", "max_tokens", "parallel_tool_calls", "presence_penalty", "reasoning", "reasoning_effort", "repetition_penalty", "response_format", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_logprobs", "top_p"},
			SupportedReasoningEfforts:   []string{"max", "xhigh", "high", "medium", "low", "minimal", "none"},
			DefaultReasoningEffort:      "medium",
			Description:                 "upstage/solar-pro4 on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
	}
}
