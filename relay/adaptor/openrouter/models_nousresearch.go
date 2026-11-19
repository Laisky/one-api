package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// nousresearchModels returns independently owned nousresearch defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func nousresearchModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"nousresearch/hermes-2-pro-llama-3-8b": {
			Ratio:                       nativeRate(0.14),
			CompletionRatio:             1,
			ContextLength:               8192,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens"},
			HuggingFaceID:               "NousResearch/Hermes-2-Pro-Llama-3-8B",
			Description:                 "Hermes 2 Pro is an upgraded, retrained version of Nous Hermes 2, consisting of an updated and cleaned version of the OpenHermes 2.5 Dataset, as well as a newly introduced... [Deprecated: no longer listed on OpenRouter's live model catalog as of 2026-07-13]",
		},
		"nousresearch/hermes-3-llama-3.1-405b": {
			Ratio:                       nativeRate(1),
			CompletionRatio:             1,
			ContextLength:               131072,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "logit_bias", "max_tokens", "min_p", "presence_penalty", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_p"},
			HuggingFaceID:               "NousResearch/Hermes-3-Llama-3.1-405B",
			Description:                 "nousresearch/hermes-3-llama-3.1-405b on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"nousresearch/hermes-3-llama-3.1-405b:free": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "stop", "max_tokens"},
			HuggingFaceID:               "NousResearch/Hermes-3-Llama-3.1-405B",
			Description:                 "Hermes 3 is a generalist language model with many improvements over Hermes 2, including advanced agentic capabilities, much better roleplaying, reasoning, multi-turn conversation, long context coherence, and improvements across the...",
		},
		"nousresearch/hermes-3-llama-3.1-70b": {
			Ratio:                       nativeRate(0.7),
			CompletionRatio:             1,
			ContextLength:               131072,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"structured_outputs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "logit_bias", "max_tokens", "min_p", "presence_penalty", "repetition_penalty", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_p"},
			HuggingFaceID:               "NousResearch/Hermes-3-Llama-3.1-70B",
			Description:                 "nousresearch/hermes-3-llama-3.1-70b on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"nousresearch/hermes-4-405b": {
			Ratio:                       nativeRate(1),
			CompletionRatio:             3,
			ContextLength:               131072,
			MaxOutputTokens:             117964,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs", "reasoning", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "logit_bias", "logprobs", "max_tokens", "presence_penalty", "reasoning", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "NousResearch/Hermes-4-405B",
			Description:                 "nousresearch/hermes-4-405b on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
			CachedInputRatio:            nativeRate(0.2),
		},
		"nousresearch/hermes-4-70b": {
			Ratio:                       nativeRate(0.13),
			CompletionRatio:             0.4 / 0.13,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "max_tokens"},
			HuggingFaceID:               "NousResearch/Hermes-4-70B",
			Description:                 "Hermes 4 70B is a hybrid reasoning model from Nous Research, built on Meta-Llama-3.1-70B. It introduces the same hybrid mode as the larger 405B release, allowing the model to either...",
		},
	}
}
