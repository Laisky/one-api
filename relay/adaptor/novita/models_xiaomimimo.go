package novita

import "github.com/Laisky/one-api/relay/adaptor"

// xiaomimimoModels returns independently owned xiaomimimo defaults for novita.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://api.novita.ai/v3/openai/models
func xiaomimimoModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"xiaomimimo/mimo-v2-flash": {
			Ratio:                       nativeRate(0.11),
			CompletionRatio:             3,
			CachedInputRatio:            nativeRate(0.024),
			ContextLength:               262144,
			MaxOutputTokens:             32000,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "XiaomiMiMo/MiMo-V2-Flash",
			Description:                 "Xiaomi MiMo V2 Flash chat model with 256K context.",
		},
		"xiaomimimo/mimo-v2.5": {
			Ratio:                       nativeRate(0.168),
			CompletionRatio:             2,
			CachedInputRatio:            nativeRate(0.0034),
			ContextLength:               1048576,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text", "image", "video"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "XiaomiMiMo/MiMo-V2.5",
			Description:                 "xiaomimimo/mimo-v2.5 on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"xiaomimimo/mimo-v2.5-pro": {
			Ratio:                       nativeRate(0.522),
			CompletionRatio:             2,
			CachedInputRatio:            nativeRate(0.0043),
			ContextLength:               1048576,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "top_k", "min_p", "max_tokens", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty", "seed", "n", "logit_bias", "logprobs", "top_logprobs", "response_format", "tools", "tool_choice"},
			Quantization:                "bf16",
			HuggingFaceID:               "XiaomiMiMo/MiMo-V2.5-Pro",
			Description:                 "xiaomimimo/mimo-v2.5-pro on novita; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"xiaomimimo/mimo-v2.6-flash": {
			Ratio:             nativeRate(0.14),
			CompletionRatio:   (0.28 * 1.0) / 0.14,
			CachedInputRatio:  nativeRate(0.0028),
			ContextLength:     1048576,
			MaxOutputTokens:   131072,
			InputModalities:   []string{"text", "image", "video", "audio"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "structured_outputs", "reasoning"},
			Description:       "xiaomimimo/mimo-v2.6-flash on Novita, verified from the public models API on 2026-10-12. Uses Novita's exact USD tariff and API identifier; no creator, priority, or clock tariff is inferred from the suffix. The source does not publish a discrete reasoning-effort vocabulary; this adaptor does not advertise one.",
		},
		"xiaomimimo/mimo-v2.6-pro": {
			Ratio:             nativeRate(0.435),
			CompletionRatio:   (0.87 * 1.0) / 0.435,
			CachedInputRatio:  nativeRate(0.0036),
			ContextLength:     1048576,
			MaxOutputTokens:   131072,
			InputModalities:   []string{"text", "image", "video", "audio"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "structured_outputs", "reasoning"},
			Description:       "xiaomimimo/mimo-v2.6-pro on Novita, verified from the public models API on 2026-10-12. Uses Novita's exact USD tariff and API identifier; no creator, priority, or clock tariff is inferred from the suffix. The source does not publish a discrete reasoning-effort vocabulary; this adaptor does not advertise one.",
		},
	}
}
