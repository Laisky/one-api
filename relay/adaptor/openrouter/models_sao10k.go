package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// sao10kModels returns independently owned sao10k defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func sao10kModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"sao10k/l3-euryale-70b": {
			Ratio:                       nativeRate(1.48),
			CompletionRatio:             1,
			ContextLength:               8192,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			HuggingFaceID:               "Sao10K/L3-70B-Euryale-v2.1",
			Description:                 "Euryale 70B v2.1 is a model focused on creative roleplay from [Sao10k](https://ko-fi.com/sao10k). - Better prompt adherence. - Better anatomy / spatial awareness. - Adapts much better to unique and custom... [Deprecated: no longer listed on OpenRouter's live model catalog as of 2026-07-13]",
		},
		"sao10k/l3-lunaris-8b": {
			Ratio:                       nativeRate(0.04),
			CompletionRatio:             1.25,
			ContextLength:               8192,
			MaxOutputTokens:             7372,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "logit_bias", "logprobs", "max_tokens", "min_p", "presence_penalty", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "Sao10K/L3-8B-Lunaris-v1",
			Description:                 "sao10k/l3-lunaris-8b on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sao10k/l3.1-70b-hanami-x1": {
			Ratio:                       nativeRate(3),
			CompletionRatio:             1,
			ContextLength:               16000,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logit_bias"},
			HuggingFaceID:               "Sao10K/L3.1-70B-Hanami-x1",
			Description:                 "This is [Sao10K](/sao10k)'s experiment over [Euryale v2.2](/sao10k/l3.1-euryale-70b).",
		},
		"sao10k/l3.1-euryale-70b": {
			Ratio:                       nativeRate(0.85),
			CompletionRatio:             1,
			ContextLength:               131072,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"structured_outputs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "logit_bias", "max_tokens", "min_p", "presence_penalty", "repetition_penalty", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_p"},
			HuggingFaceID:               "Sao10K/L3.1-70B-Euryale-v2.2",
			Description:                 "sao10k/l3.1-euryale-70b on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"sao10k/l3.3-euryale-70b": {
			Ratio:                       nativeRate(0.65),
			CompletionRatio:             0.75 / 0.65,
			ContextLength:               131072,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "logprobs", "max_tokens", "presence_penalty", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "top_logprobs", "top_p"},
			HuggingFaceID:               "Sao10K/L3.3-70B-Euryale-v2.3",
			Description:                 "sao10k/l3.3-euryale-70b on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
