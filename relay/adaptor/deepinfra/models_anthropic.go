package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// anthropicModels returns independently owned anthropic defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func anthropicModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"anthropic/claude-fable-5": {
			Ratio:                       nativeRate(10),
			CompletionRatio:             5,
			ContextLength:               1000000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "anthropic/claude-fable-5 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"anthropic/claude-haiku-4-5": {
			Ratio:                       nativeRate(1),
			CompletionRatio:             5,
			ContextLength:               200000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "anthropic/claude-haiku-4-5 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"anthropic/claude-opus-4-7": {
			Ratio:                       nativeRate(5),
			CompletionRatio:             5,
			ContextLength:               1000000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "anthropic/claude-opus-4-7 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"anthropic/claude-opus-4-8": {
			Ratio:                       nativeRate(5),
			CompletionRatio:             5,
			ContextLength:               1000000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "anthropic/claude-opus-4-8 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"anthropic/claude-opus-5": {
			Ratio:                       nativeRate(5),
			CompletionRatio:             5,
			ContextLength:               1000000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "anthropic/claude-opus-5 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"anthropic/claude-opus-5-5": {
			Ratio:             nativeRate(4),
			CompletionRatio:   5,
			CachedInputRatio:  nativeRate(0.2),
			ContextLength:     1000000,
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Description:       "anthropic/claude-opus-5-5 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"anthropic/claude-sonnet-4-6": {
			Ratio:                       nativeRate(3),
			CompletionRatio:             5,
			ContextLength:               1000000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "anthropic/claude-sonnet-4-6 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"anthropic/claude-sonnet-5": {
			Ratio:                       nativeRate(3),
			CompletionRatio:             5,
			ContextLength:               1000000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "anthropic/claude-sonnet-5 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"anthropic/claude-sonnet-5-5": {
			Ratio:             nativeRate(2),
			CompletionRatio:   (10 * 1.0) / 2,
			ContextLength:     1000000,
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			MaxOutputTokens:   128000,
			Description:       "anthropic/claude-sonnet-5-5 on DeepInfra; public host catalogue verified 2026-10-12. Prices and limits are host-specific, not copied from another provider. The 128K output limit is stated on DeepInfra's own model page (its API leaves max_output_tokens null; rechecked 2026-11-18); cache prices, sampling controls and reasoning-effort levels remain unspecified. This is a public partner-hosted endpoint; free anonymous access is not advertised. No native Anthropic cache tariff is inferred.",
		},
	}
}
