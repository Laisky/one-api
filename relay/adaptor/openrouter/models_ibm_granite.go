package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// ibmGraniteModels returns independently owned ibm granite defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func ibmGraniteModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"ibm-granite/granite-4.0-h-micro": {
			Ratio:                       nativeRate(0.017),
			CompletionRatio:             0.112 / 0.017,
			ContextLength:               131000,
			MaxOutputTokens:             117900,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "logit_bias", "logprobs", "max_tokens", "min_p", "presence_penalty", "repetition_penalty", "response_format", "seed", "stop", "temperature", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "ibm-granite/granite-4.0-h-micro",
			Description:                 "ibm-granite/granite-4.0-h-micro on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"ibm-granite/granite-4.1-8b": {
			Ratio:                       nativeRate(0.05),
			CompletionRatio:             2,
			CachedInputRatio:            nativeRate(0.05),
			ContextLength:               131072,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "seed", "stop", "max_tokens"},
			HuggingFaceID:               "ibm-granite/granite-4.1-8b",
			Description:                 "Granite 4.1 8B is a dense, decoder-only 8-billion-parameter language model from IBM, part of the Granite 4.1 family. It supports a 131K-token context window and is designed for enterprise tasks.",
		},
		"ibm-granite/granite-4.2-8b": {
			Ratio:                       nativeRate(0.06),
			CompletionRatio:             2.5,
			CachedInputRatio:            nativeRate(0.015),
			ContextLength:               131072,
			MaxOutputTokens:             117964,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logit_bias", "logprobs", "max_tokens", "min_p", "presence_penalty", "reasoning", "reasoning_effort", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_logprobs", "top_p"},
			SupportedReasoningEfforts:   []string{"high", "low", "none"},
			DefaultReasoningEffort:      "high",
			HuggingFaceID:               "ibm-granite/granite-4.2-8b",
			Description:                 "ibm-granite/granite-4.2-8b on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
	}
}
