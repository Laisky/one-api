package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// openaiModels returns the openai model defaults for deepinfra.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func openaiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"openai/gpt-oss-120b": {
			Ratio:                       nativeRate(0.037),
			CompletionRatio:             4.594594594594595,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "openai/gpt-oss-120b on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"openai/gpt-oss-120b-Turbo": {
			Ratio:                       nativeRate(0.15),
			CompletionRatio:             4,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "openai/gpt-oss-120b-Turbo on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"openai/gpt-oss-120b-Ultra": {
			Ratio:                       nativeRate(0.2),
			CompletionRatio:             4.75,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "openai/gpt-oss-120b-Ultra on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"openai/gpt-oss-20b": {
			Ratio:                       nativeRate(0.030000000000000002),
			CompletionRatio:             4.666666666666666,
			ContextLength:               131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "stop", "seed", "max_tokens", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice", "n"},
			Description:                 "openai/gpt-oss-20b on deepinfra; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"openai/whisper-large-v3": {
			Ratio:           nativeRate(0.75),
			CompletionRatio: 1,
			Audio: &adaptor.AudioPricingConfig{
				PromptTokensPerSecond: 10,
				UsdPerSecond:          7.5e-06,
			},
			InputModalities:  []string{"audio"},
			OutputModalities: []string{"text"},
			Description:      "Whisper large v3 transcription and translation model.",
		},
		"openai/whisper-large-v3-turbo": {
			Ratio:           nativeRate(0.3333333333333333),
			CompletionRatio: 1,
			Audio: &adaptor.AudioPricingConfig{
				PromptTokensPerSecond: 10,
				UsdPerSecond:          3.3333333333333333e-06,
			},
			InputModalities:  []string{"audio"},
			OutputModalities: []string{"text"},
			Description:      "Faster Whisper large v3 transcription and translation model.",
		},
	}
}
