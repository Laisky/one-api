package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// moonshotaiModels returns independently owned moonshotai defaults for togetherai.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://docs.together.ai/docs/serverless/models
func moonshotaiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"moonshotai/Kimi-K2.5": {
			Ratio:                       nativeRate(0.5),
			CompletionRatio:             5.6,
			ContextLength:               262144,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp4",
			HuggingFaceID:               "moonshotai/Kimi-K2-Instruct",
			Description:                 "Kimi K2.5 agentic MoE (1T/32B active) with vision support and 262K context. (RETIRED 2026-05-21; use moonshotai/Kimi-K2.6)",
		},
		"moonshotai/Kimi-K2.6": {
			Ratio:                       nativeRate(1.2),
			CompletionRatio:             3.75,
			CachedInputRatio:            nativeRate(0.2),
			ContextLength:               262144,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp4",
			HuggingFaceID:               "moonshotai/Kimi-K2-Instruct",
			Description:                 "Kimi K2.6 agentic MoE (1T/32B active) with cached input pricing, optimised for tool use and SWE.",
		},
		"moonshotai/Kimi-K2.7-Code": {
			Ratio:                       nativeRate(0.95),
			CompletionRatio:             4.0 / 0.95,
			CachedInputRatio:            nativeRate(0.19),
			ContextLength:               262144,
			MaxOutputTokens:             32768,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			HuggingFaceID:               "moonshotai/Kimi-K2.7-Code",
			Description:                 "Moonshot Kimi K2.7 Code multimodal coding model, 256K context, $0.95/$4.00 per 1M.",
		},
		"moonshotai/Kimi-K3": {
			Ratio:             nativeRate(3),
			CompletionRatio:   (15 * 1.0) / 3,
			CachedInputRatio:  nativeRate(0.3),
			ContextLength:     1048576,
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
			Description:       "moonshotai/Kimi-K3 on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
		},
		"moonshotai/Kimi-K3.1-Pro": {
			Ratio:             nativeRate(2),
			CompletionRatio:   5,
			CachedInputRatio:  nativeRate(0.2),
			ContextLength:     1048576,
			SupportedFeatures: []string{"tools", "structured_outputs"},
			Quantization:      "fp4",
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			Description:       "moonshotai/Kimi-K3.1-Pro on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
		},
	}
}
