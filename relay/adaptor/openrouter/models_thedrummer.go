package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// thedrummerModels returns independently owned thedrummer defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func thedrummerModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"thedrummer/cydonia-24b-v4.1": {
			Ratio:                       nativeRate(0.3),
			CompletionRatio:             0.5 / 0.3,
			CachedInputRatio:            nativeRate(0.15),
			ContextLength:               131072,
			MaxOutputTokens:             117964,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "logit_bias", "logprobs", "max_tokens", "presence_penalty", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "thedrummer/cydonia-24b-v4.1",
			Description:                 "thedrummer/cydonia-24b-v4.1 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"thedrummer/rocinante-12b": {
			Ratio:                       nativeRate(0.25),
			CompletionRatio:             2,
			ContextLength:               65536,
			MaxOutputTokens:             65536,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "logprobs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logit_bias"},
			HuggingFaceID:               "TheDrummer/Rocinante-12B-v1.1",
			Description:                 "Rocinante 12B is designed for engaging storytelling and rich prose. Early testers have reported: - Expanded vocabulary with unique and expressive word choices - Enhanced creativity for vivid narratives -...",
		},
		"thedrummer/skyfall-36b-v2": {
			Ratio:                       nativeRate(0.55),
			CompletionRatio:             0.8 / 0.55,
			CachedInputRatio:            nativeRate(0.25),
			ContextLength:               32768,
			MaxOutputTokens:             29491,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "logit_bias", "logprobs", "max_tokens", "presence_penalty", "repetition_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "TheDrummer/Skyfall-36B-v2",
			Description:                 "thedrummer/skyfall-36b-v2 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"thedrummer/unslopnemo-12b": {
			Ratio:                       nativeRate(0.4),
			CompletionRatio:             1,
			ContextLength:               1024000,
			MaxOutputTokens:             819200,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"structured_outputs", "logprobs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "logit_bias", "logprobs", "max_tokens", "presence_penalty", "repetition_penalty", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_logprobs", "top_p"},
			HuggingFaceID:               "TheDrummer/UnslopNemo-12B-v4.1",
			Description:                 "thedrummer/unslopnemo-12b on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
