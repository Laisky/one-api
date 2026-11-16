package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// zaiOrgModels returns independently owned zai org defaults for togetherai.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://docs.together.ai/docs/serverless/models
func zaiOrgModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"zai-org/GLM-5": {
			Ratio:                       nativeRate(1),
			CompletionRatio:             3.2,
			ContextLength:               202752,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp4",
			HuggingFaceID:               "zai-org/GLM-5",
			Description:                 "Z.ai GLM-5 hybrid-reasoning MoE (754B/40B active), state-of-the-art open-source agentic model. (RETIRED — not available on Together's Serverless API as of 2026-07-13; use zai-org/GLM-5.1 or zai-org/GLM-5.2)",
		},
		"zai-org/GLM-5.1": {
			Ratio:                       nativeRate(1.4),
			CompletionRatio:             3.142857142857143,
			CachedInputRatio:            nativeRate(0.26),
			ContextLength:               202752,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp4",
			HuggingFaceID:               "zai-org/GLM-5.1",
			Description:                 "Z.ai GLM-5.1 hybrid-reasoning MoE (754B/40B active) with strong agentic performance.",
		},
		"zai-org/GLM-5.2": {
			Ratio:                       nativeRate(1.4),
			CompletionRatio:             (4.4 * 1.0) / 1.4,
			CachedInputRatio:            nativeRate(0.26),
			ContextLength:               1048575,
			MaxOutputTokens:             131072,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp4",
			HuggingFaceID:               "zai-org/GLM-5.2",
			Description:                 "zai-org/GLM-5.2 on Together's public serverless API; verified 2026-10-12. Token tariffs are USD per million. Omitted source fields retain prior metadata rather than becoming invented limits or free prices.",
		},
		"zai-org/GLM-5.3": {
			Ratio:             nativeRate(1.4),
			CompletionRatio:   (4.4 * 1.0) / 1.4,
			CachedInputRatio:  nativeRate(0.26),
			ContextLength:     1048575,
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
			Description:       "zai-org/GLM-5.3 on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
			Quantization:      "fp4",
			OutputModalities:  []string{"text"},
		},
		"zai-org/GLM-5.3-Flash": {
			Ratio:             nativeRate(0.15),
			CompletionRatio:   (0.5 * 1.0) / 0.15,
			CachedInputRatio:  nativeRate(0.03),
			ContextLength:     1048575,
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
			Description:       "zai-org/GLM-5.3-Flash on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
			Quantization:      "fp4",
			OutputModalities:  []string{"text"},
		},
	}
}
