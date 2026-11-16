package novita

import "github.com/Laisky/one-api/relay/adaptor"

// sao10kModels returns independently owned sao10k defaults for novita.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.novita.ai/v3/openai/models
func sao10kModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"Sao10K/L3-8B-Stheno-v3.2": {
			Ratio:                       nativeRate(0.05),
			CompletionRatio:             1,
			ContextLength:               8192,
			MaxOutputTokens:             32000,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "Sao10K/L3-8B-Stheno-v3.2",
			Description:                 "Sao10K/L3-8B-Stheno-v3.2 on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sao10k/l3-70b-euryale-v2.1": {
			Ratio:             nativeRate(1.48),
			CompletionRatio:   1,
			ContextLength:     8192,
			MaxOutputTokens:   8192,
			InputModalities:   []string{"text"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools"},
			Description:       "sao10k/l3-70b-euryale-v2.1 on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sao10k/l3-8b-lunaris": {
			Ratio:                       nativeRate(0.05),
			CompletionRatio:             1,
			ContextLength:               8192,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "Sao10K/L3-8B-Lunaris-v1",
			Description:                 "sao10k/l3-8b-lunaris on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"sao10k/l31-70b-euryale-v2.2": {
			Ratio:                       nativeRate(1.48),
			CompletionRatio:             1,
			ContextLength:               8192,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "Sao10K/L3.1-70B-Euryale-v2.2",
			Description:                 "sao10k/l31-70b-euryale-v2.2 on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
