package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// meta_llamaModels returns the meta_llama model defaults for deepinfra.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func meta_llamaModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"meta-llama/Llama-3.3-70B-Instruct-Turbo": {
			Ratio:                       nativeRate(0.1),
			CompletionRatio:             3.1999999999999997,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "meta-llama/Llama-3.3-70B-Instruct-Turbo on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8": {
			Ratio:                       nativeRate(0.2),
			CompletionRatio:             4,
			ContextLength:               1048576,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8 on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"meta-llama/Llama-4-Scout-17B-16E-Instruct": {
			Ratio:                       nativeRate(0.1),
			CompletionRatio:             2.9999999999999996,
			ContextLength:               327680,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "meta-llama/Llama-4-Scout-17B-16E-Instruct on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"meta-llama/Llama-Guard-4-12B": {
			Ratio:                       nativeRate(0.18),
			CompletionRatio:             1,
			ContextLength:               163840,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "meta-llama/Llama-Guard-4-12B on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo": {
			Ratio:                       nativeRate(0.4),
			CompletionRatio:             1,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo": {
			Ratio:                       nativeRate(0.02),
			CompletionRatio:             2,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
