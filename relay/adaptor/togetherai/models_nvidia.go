package togetherai

import "github.com/Laisky/one-api/relay/adaptor"

// nvidiaModels returns independently owned nvidia defaults for togetherai.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://docs.together.ai/docs/serverless/models
func nvidiaModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
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
		"nvidia/nemotron-3-asr-streaming-0.6b": {
			Ratio:           0,
			CompletionRatio: 1,
			Audio: &adaptor.AudioPricingConfig{
				UsdPerSecond: 2.5e-05,
			},
		},
		"nvidia/nemotron-3-ultra-550b-a55b": {
			Ratio:                       nativeRate(0.6),
			CompletionRatio:             6,
			CachedInputRatio:            nativeRate(0.2),
			ContextLength:               512300,
			MaxOutputTokens:             32768,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "seed", "max_tokens"},
			HuggingFaceID:               "nvidia/Nemotron-3-Ultra-550B-A55B",
			Description:                 "NVIDIA Nemotron-3 Ultra 550B A55B reasoning MoE, 512K context, $0.60/$3.60 per 1M.",
		},
		"nvidia/nemotron-3.5-asr-streaming-0.6b": {
			Ratio:           0,
			CompletionRatio: 1,
			Audio: &adaptor.AudioPricingConfig{
				UsdPerSecond: 2.5e-05,
			},
		},
		"nvidia/parakeet-tdt-0.6b-v3": {
			Ratio:           0,
			CompletionRatio: 1,
			Audio: &adaptor.AudioPricingConfig{
				UsdPerSecond: 2.5e-05,
			},
		},
	}
}
