package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// perplexityModels returns independently owned perplexity defaults for openrouter.
// It takes no arguments and keeps each model definition directly editable in its family.
// Source: https://openrouter.ai/api/v1/models
func perplexityModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"perplexity/sonar": {
			Ratio:                       nativeRate(1),
			CompletionRatio:             1,
			ContextLength:               127072,
			MaxOutputTokens:             114364,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"web_search"},
			SupportedSamplingParameters: []string{"frequency_penalty", "max_tokens", "presence_penalty", "temperature", "top_k", "top_p", "web_search_options"},
			Description:                 "perplexity/sonar on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"perplexity/sonar-deep-research": {
			Ratio:                       nativeRate(2),
			CompletionRatio:             4,
			ContextLength:               128000,
			MaxOutputTokens:             115200,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"reasoning", "web_search"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "max_tokens", "presence_penalty", "reasoning", "temperature", "top_k", "top_p", "web_search_options"},
			Description:                 "perplexity/sonar-deep-research on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"perplexity/sonar-pro": {
			Ratio:                       nativeRate(3),
			CompletionRatio:             5,
			ContextLength:               200000,
			MaxOutputTokens:             8000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"web_search"},
			SupportedSamplingParameters: []string{"frequency_penalty", "max_tokens", "presence_penalty", "temperature", "top_k", "top_p", "web_search_options"},
			Description:                 "perplexity/sonar-pro on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"perplexity/sonar-pro-search": {
			Ratio:                       nativeRate(3),
			CompletionRatio:             5,
			ContextLength:               200000,
			MaxOutputTokens:             8000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"structured_outputs", "reasoning", "web_search"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "max_tokens", "presence_penalty", "reasoning", "structured_outputs", "temperature", "top_k", "top_p", "web_search_options"},
			Description:                 "perplexity/sonar-pro-search on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
		"perplexity/sonar-reasoning-pro": {
			Ratio:                       nativeRate(2),
			CompletionRatio:             4,
			ContextLength:               128000,
			MaxOutputTokens:             115200,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"reasoning", "web_search"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "max_tokens", "presence_penalty", "reasoning", "temperature", "top_k", "top_p", "web_search_options"},
			Description:                 "perplexity/sonar-reasoning-pro on OpenRouter; public routing-default token tariff and metadata checked 2026-11-16. Provider selection and separately metered tools can change charges.",
		},
	}
}
