package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// novemberChatModels1 returns independent host-specific chat defaults checked 2026-11-16.
// It takes no parameters and returns static Go configurations; account limits remain provider-controlled.
// Source: https://docs.together.ai/docs/serverless/models
func novemberChatModels1() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"deepseek-ai/DeepSeek-V4.1-Flash-1017": {
			Ratio:             nativeRate(0.3),
			CompletionRatio:   4,
			CachedInputRatio:  nativeRate(0.006),
			ContextLength:     1000000,
			SupportedFeatures: []string{"tools", "structured_outputs"},
			Quantization:      "fp8",
			OutputModalities:  []string{"text"},
			Description:       "deepseek-ai/DeepSeek-V4.1-Flash-1017 on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
		},
		"elorian-labs/glimpse-1.0": {
			Ratio:             nativeRate(0.6),
			CompletionRatio:   5,
			CachedInputRatio:  nativeRate(0.06),
			ContextLength:     262144,
			SupportedFeatures: []string{"tools", "structured_outputs"},
			Quantization:      "fp8",
			OutputModalities:  []string{"text"},
			Description:       "elorian-labs/glimpse-1.0 on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
		},
		"meta-models/Muse-Spark-Open-816B-A42B": {
			Ratio:             nativeRate(1.25),
			CompletionRatio:   3.4,
			CachedInputRatio:  nativeRate(0.15),
			ContextLength:     1048576,
			SupportedFeatures: []string{"tools", "structured_outputs"},
			Quantization:      "fp4",
			OutputModalities:  []string{"text"},
			Description:       "meta-models/Muse-Spark-Open-816B-A42B on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
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
		"nvidia/NVIDIA-Nemotron-3.5-Super-VL": {
			Ratio:             nativeRate(0.15),
			CompletionRatio:   3.33333333333333,
			CachedInputRatio:  nativeRate(0.02),
			ContextLength:     262144,
			SupportedFeatures: []string{"tools", "structured_outputs"},
			Quantization:      "fp4",
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			Description:       "nvidia/NVIDIA-Nemotron-3.5-Super-VL on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
		},
		"reflection/Beam-1.0-501B-A23B": {
			Ratio:             nativeRate(0.4),
			CompletionRatio:   3,
			CachedInputRatio:  nativeRate(0.04),
			ContextLength:     1048576,
			SupportedFeatures: []string{"tools", "structured_outputs"},
			Quantization:      "fp4",
			OutputModalities:  []string{"text"},
			Description:       "reflection/Beam-1.0-501B-A23B on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
		},
		"ricursive-ai/Ripple-65B-Preview": {
			Ratio:            nativeRate(0.2),
			CompletionRatio:  3,
			ContextLength:    131072,
			OutputModalities: []string{"text"},
			Description:      "ricursive-ai/Ripple-65B-Preview on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
		},
		"thinkingmachines/Inkling-2-Small": {
			Ratio:             nativeRate(0.3),
			CompletionRatio:   3.33333333333333,
			CachedInputRatio:  nativeRate(0.03),
			ContextLength:     1048576,
			SupportedFeatures: []string{"tools", "structured_outputs"},
			Quantization:      "fp4",
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			Description:       "thinkingmachines/Inkling-2-Small on Together Standard serverless; published token prices and metadata checked 2026-11-16. Dedicated deployments and US-only endpoint surcharges are separate contracts. Unpublished output limits remain unspecified.",
		},
	}
}
