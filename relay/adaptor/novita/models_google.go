package novita

import "github.com/Laisky/one-api/relay/adaptor"

// googleModels returns independently owned google defaults for novita.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.novita.ai/v3/openai/models
func googleModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"google/gemma-3-12b-it": {
			Ratio:                       nativeRate(0.05),
			CompletionRatio:             2,
			ContextLength:               131072,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "google/gemma-3-12b-it",
			Description:                 "google/gemma-3-12b-it on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"google/gemma-3-27b-it": {
			Ratio:                       nativeRate(0.119),
			CompletionRatio:             0.2 / 0.119,
			ContextLength:               98304,
			MaxOutputTokens:             16384,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "google/gemma-3-27b-it",
			Description:                 "google/gemma-3-27b-it on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"google/gemma-4-26b-a4b-it": {
			Ratio:                       nativeRate(0.13),
			CompletionRatio:             0.4 / 0.13,
			ContextLength:               262144,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "google/gemma-4-26b-a4b-it",
			Description:                 "google/gemma-4-26b-a4b-it on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"google/gemma-4-31b-it": {
			Ratio:                       nativeRate(0.14),
			CompletionRatio:             0.4 / 0.14,
			ContextLength:               262144,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "google/gemma-4-31b-it",
			Description:                 "google/gemma-4-31b-it on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
