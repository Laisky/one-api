package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// sonnet55Models returns OpenRouter's independently published Sonnet 5.5 entry.
// It takes no arguments and returns fresh, directly editable model definitions.
// Source (2026-09-28): https://openrouter.ai/api/v1/models
// Use the public id, not canonical_slug. The separately listed :batch variant
// is not advertised without qualifying the gateway's asynchronous Batch path.
func sonnet55Models() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"anthropic/claude-sonnet-5.5": {
			Ratio:             nativeRate(2),
			CompletionRatio:   5,
			CachedInputRatio:  nativeRate(0.2),
			CacheWrite5mRatio: nativeRate(2.5),
			CacheWrite1hRatio: nativeRate(4),
			ContextLength:     1000000,
			MaxOutputTokens:   128000,
			InputModalities:   []string{"text", "image", "file"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			// These are OpenRouter's advertised controls, not Anthropic's wire allowlist.
			SupportedSamplingParameters: []string{"include_reasoning", "max_completion_tokens", "max_tokens", "reasoning", "reasoning_effort", "response_format", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "verbosity"},
			SupportedReasoningEfforts:   []string{"max", "xhigh", "high", "medium", "low"},
			DefaultReasoningEffort:      "high",
			Description:                 "Claude Sonnet 5.5 on OpenRouter; official catalog reviewed 2026-09-28. 1M context, 128K output, mandatory reasoning (default high). Standard input/output $2/$10 per million tokens, cache reads $0.20 and 5m/1h writes $2.50/$4. Provider routing and account access remain operator-controlled; this entry does not enable the Batch variant.",
		},
	}
}
