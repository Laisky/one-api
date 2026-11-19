package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// moonshotaiModels returns independently owned moonshotai defaults for deepinfra.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.deepinfra.com/models/list
func moonshotaiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"moonshotai/Kimi-K2.5": {
			Ratio:                       nativeRate(0.45),
			CompletionRatio:             5,
			CachedInputRatio:            nativeRate(0.07),
			ContextLength:               262144,
			MaxOutputTokens:             64000,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "Kimi K2.5 general reasoning model.",
		},
		"moonshotai/Kimi-K2.6": {
			Ratio:                       nativeRate(0.75),
			CompletionRatio:             3.5 / 0.75,
			CachedInputRatio:            nativeRate(0.15),
			ContextLength:               262144,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "moonshotai/Kimi-K2.6 on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified.",
			Quantization:                "fp4",
		},
		"moonshotai/Kimi-K2.7-Code": {
			Ratio:                       nativeRate(0.68),
			CompletionRatio:             5,
			CachedInputRatio:            nativeRate(0.136),
			ContextLength:               262144,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "Kimi K2.7 optimized for software engineering.",
		},
		"moonshotai/Kimi-K3": {
			Ratio:                       nativeRate(2.85),
			CompletionRatio:             5,
			CachedInputRatio:            nativeRate(0.285),
			ContextLength:               1048576,
			MaxOutputTokens:             1048576,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "moonshotai/Kimi-K3 on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Published output limit checked 2026-11-18; reasoning controls remain unspecified. Explicit cache writes are published at 1.25x/2x input for 5m/1h; TTL-specific request and receipt billing requires separate qualification.",
			Quantization:                "mxfp4",
		},
		"moonshotai/Kimi-K3.1-Pro": {
			Ratio:             nativeRate(1.9),
			CompletionRatio:   5,
			CachedInputRatio:  nativeRate(0.19),
			ContextLength:     1048576,
			MaxOutputTokens:   943718,
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Description:       "moonshotai/Kimi-K3.1-Pro on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Published output limit checked 2026-11-18; reasoning controls remain unspecified. Includes the published 5% discount; no expiry is invented.",
		},
	}
}
