package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// gpt61SolModels returns independently owned OpenRouter GPT-6.1 Sol defaults.
// Prices, IDs, input modalities, effort levels, and the inclusive minimum-input
// override come from https://openrouter.ai/api/v1/models, checked 2026-09-30.
// Pro is OpenRouter's reasoning.mode=pro alias, not a native OpenAI model ID.
// Batch variants require the asynchronous Batch API and are not advertised by
// this synchronous adaptor. No dated canonical_slug is promoted to a model ID.
func gpt61SolModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"openai/gpt-6.1-sol": {
			Ratio:             nativeRate(2),
			CompletionRatio:   10.0 / 2.0,
			CachedInputRatio:  nativeRate(0.1),
			CacheWrite5mRatio: nativeRate(2.5),
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(4),
				CompletionRatio:     15.0 / 4.0,
				CachedInputRatio:    nativeRate(0.2),
				CacheWrite5mRatio:   nativeRate(5),
				InputTokenThreshold: 272_000,
			}},
			ContextLength:               1_050_000,
			MaxOutputTokens:             128_000,
			InputModalities:             []string{"file", "image", "text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"include_reasoning", "max_completion_tokens", "max_tokens", "reasoning", "reasoning_effort", "response_format", "seed", "structured_outputs", "tool_choice", "tools", "verbosity"},
			SupportedReasoningEfforts:   []string{"max", "xhigh", "high", "medium", "low"},
			DefaultReasoningEffort:      "medium",
			Description:                 "openai/gpt-6.1-sol on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"openai/gpt-6.1-sol-pro": {
			Ratio:             nativeRate(2),
			CompletionRatio:   10.0 / 2.0,
			CachedInputRatio:  nativeRate(0.1),
			CacheWrite5mRatio: nativeRate(2.5),
			Tiers: []adaptor.ModelRatioTier{{
				Ratio:               nativeRate(4),
				CompletionRatio:     15.0 / 4.0,
				CachedInputRatio:    nativeRate(0.2),
				CacheWrite5mRatio:   nativeRate(5),
				InputTokenThreshold: 272_000,
			}},
			ContextLength:               1_050_000,
			MaxOutputTokens:             128_000,
			InputModalities:             []string{"file", "image", "text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"include_reasoning", "max_completion_tokens", "max_tokens", "reasoning", "reasoning_effort", "response_format", "seed", "structured_outputs", "tool_choice", "tools", "verbosity"},
			SupportedReasoningEfforts:   []string{"max", "xhigh", "high", "medium", "low"},
			DefaultReasoningEffort:      "medium",
			Description:                 "GPT-6.1 Sol Pro on OpenRouter: the Sol model served with reasoning.mode=pro. The same per-token Standard rates apply, but Pro may consume substantially more reasoning tokens. This alias is not a native OpenAI model ID. Catalog and parameters checked 2026-11-16.",
		},
	}
}
