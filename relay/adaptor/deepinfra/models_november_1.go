package deepinfra

import "github.com/Laisky/one-api/relay/adaptor"

// novemberChatModels1 returns independent host-specific chat defaults checked 2026-11-16.
// It takes no parameters and returns static Go configurations; account limits remain provider-controlled.
// Source: https://api.deepinfra.com/models/list
func novemberChatModels1() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"deepseek-ai/DeepSeek-V4.1-Flash-1017": {
			Ratio:             nativeRate(0.098),
			CompletionRatio:   3,
			CachedInputRatio:  nativeRate(0.00294),
			ContextLength:     1048576,
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Quantization:      "fp8",
			Description:       "deepseek-ai/DeepSeek-V4.1-Flash-1017 on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified. Includes the published 51% discount; no expiry is invented.",
		},
		"Qwen/Qwen4-Max": {
			Ratio:             nativeRate(1.413),
			CompletionRatio:   3.00070771408351,
			CachedInputRatio:  nativeRate(0.0710000001),
			ContextLength:     1000000,
			InputModalities:   []string{"text", "image", "video"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Description:       "Qwen/Qwen4-Max on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified.",
		},
		"moonshotai/Kimi-K3.1-Pro": {
			Ratio:             nativeRate(1.9),
			CompletionRatio:   5,
			CachedInputRatio:  nativeRate(0.19),
			ContextLength:     1048576,
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Description:       "moonshotai/Kimi-K3.1-Pro on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified. Includes the published 5% discount; no expiry is invented.",
		},
		"nvidia/NVIDIA-Nemotron-3.5-Super-VL": {
			Ratio:             nativeRate(0.15),
			CompletionRatio:   3.33333333333333,
			CachedInputRatio:  nativeRate(0.0199999995),
			ContextLength:     262144,
			InputModalities:   []string{"text", "image", "video"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Quantization:      "fp4",
			Description:       "nvidia/NVIDIA-Nemotron-3.5-Super-VL on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified.",
		},
		"thinkingmachines/Inkling-2-Small": {
			Ratio:             nativeRate(0.285),
			CompletionRatio:   3.33333333333333,
			CachedInputRatio:  nativeRate(0.0285),
			ContextLength:     1048576,
			InputModalities:   []string{"text", "image"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			Quantization:      "fp4",
			Description:       "thinkingmachines/Inkling-2-Small on DeepInfra; Standard host token tariff and capability tags checked 2026-11-16. Unpublished output limits and reasoning controls remain unspecified. Includes the published 5% discount; no expiry is invented.",
		},
	}
}
