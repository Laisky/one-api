package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// sakanaModels returns independently owned sakana defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func sakanaModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"sakana/fugu-max": {
			Ratio:                       nativeRate(2),
			CompletionRatio:             3,
			CachedInputRatio:            nativeRate(0.25),
			ContextLength:               1000000,
			MaxOutputTokens:             128000,
			InputModalities:             []string{"text", "image", "file"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "structured_outputs", "reasoning", "web_search"},
			SupportedSamplingParameters: []string{"include_reasoning", "reasoning", "reasoning_effort", "structured_outputs", "tool_choice", "tools", "web_search_options"},
			SupportedReasoningEfforts:   []string{"max", "xhigh", "high"},
			DefaultReasoningEffort:      "high",
			Description:                 "sakana/fugu-max on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"sakana/fugu-ultra": {
			Ratio:            nativeRate(5),
			CompletionRatio:  6,
			CachedInputRatio: nativeRate(0.5),
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(1e+01),
				CompletionRatio:     4.5,
				CachedInputRatio:    nativeRate(1),
				InputTokenThreshold: 272000,
			},
			},
			ContextLength:               1000000,
			MaxOutputTokens:             128000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "structured_outputs", "reasoning", "web_search"},
			SupportedSamplingParameters: []string{"include_reasoning", "reasoning", "reasoning_effort", "structured_outputs", "tool_choice", "tools", "web_search_options"},
			SupportedReasoningEfforts:   []string{"max", "xhigh", "high"},
			DefaultReasoningEffort:      "xhigh",
			Description:                 "sakana/fugu-ultra on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"sakana/fugu-ultra-v2": {
			Ratio:            nativeRate(5),
			CompletionRatio:  6,
			CachedInputRatio: nativeRate(0.5),
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(1e+01),
				CompletionRatio:     4.5,
				CachedInputRatio:    nativeRate(1),
				InputTokenThreshold: 272000,
			},
			},
			ContextLength:               1000000,
			MaxOutputTokens:             128000,
			InputModalities:             []string{"text", "image", "file"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "structured_outputs", "reasoning", "web_search"},
			SupportedSamplingParameters: []string{"include_reasoning", "reasoning", "reasoning_effort", "structured_outputs", "tool_choice", "tools", "web_search_options"},
			SupportedReasoningEfforts:   []string{"max", "xhigh", "high"},
			DefaultReasoningEffort:      "xhigh",
			Description:                 "sakana/fugu-ultra-v2 on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"sakana/sakana-namazu": {
			Ratio:                       nativeRate(0.95),
			CompletionRatio:             4.2105263157894735,
			CachedInputRatio:            nativeRate(0.15),
			ContextLength:               262144,
			MaxOutputTokens:             65536,
			InputModalities:             []string{"text", "image", "file"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "structured_outputs", "reasoning", "web_search"},
			SupportedSamplingParameters: []string{"include_reasoning", "reasoning", "reasoning_effort", "structured_outputs", "tool_choice", "tools", "web_search_options"},
			SupportedReasoningEfforts:   []string{"high", "none"},
			DefaultReasoningEffort:      "high",
			Description:                 "sakana/sakana-namazu on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
	}
}
