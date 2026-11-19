package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// minimaxaiModels returns independently owned minimaxai defaults for togetherai.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://docs.together.ai/docs/serverless/models
func minimaxaiModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"MiniMaxAI/MiniMax-M2.5": {
			Ratio:                       nativeRate(0.3),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.06),
			ContextLength:               202752,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp4",
			HuggingFaceID:               "MiniMaxAI/MiniMax-M2",
			Description:                 "Earlier checkpoint of MiniMax M2 interleaved-thinking MoE.",
		},
		"MiniMaxAI/MiniMax-M2.7": {
			Ratio:                       nativeRate(0.3),
			CompletionRatio:             4,
			CachedInputRatio:            nativeRate(0.06),
			ContextLength:               202752,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp4",
			HuggingFaceID:               "MiniMaxAI/MiniMax-M2",
			Description:                 "MiniMax M2 interleaved-thinking MoE (230B/10B active) for coding and agentic workflows.",
		},
		"MiniMaxAI/MiniMax-M3": {
			Ratio:                       nativeRate(0.3),
			CompletionRatio:             (1.2 * 1.0) / 0.3,
			CachedInputRatio:            nativeRate(0.06),
			ContextLength:               524288,
			MaxOutputTokens:             524288,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning", "structured_outputs"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			Quantization:                "fp4",
			HuggingFaceID:               "MiniMaxAI/MiniMax-M3",
			Description:                 "MiniMaxAI/MiniMax-M3 on Together's public serverless API; verified 2026-10-12. Token tariffs are USD per million. Omitted source fields retain prior metadata rather than becoming invented limits or free prices.",
		},
		"MiniMaxAI/MiniMax-M3.1-Flash": {
			Ratio:             nativeRate(0.15),
			CompletionRatio:   4,
			CachedInputRatio:  nativeRate(0.015),
			ContextLength:     1048576,
			SupportedFeatures: []string{"tools", "structured_outputs"},
			Quantization:      "fp4",
			OutputModalities:  []string{"text"},
			Description:       "MiniMaxAI/MiniMax-M3.1-Flash on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
		},
	}
}
