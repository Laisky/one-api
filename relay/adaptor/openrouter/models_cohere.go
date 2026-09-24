package openrouter

import "github.com/Laisky/one-api/relay/adaptor"

// cohereModels returns the cohere model defaults for openrouter.
// It takes no arguments and returns independently owned, directly editable Go configurations.
func cohereModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"cohere/command-a": {
			Ratio:                       nativeRate(2.5),
			CompletionRatio:             4,
			ContextLength:               256000,
			MaxOutputTokens:             8192,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "max_tokens", "presence_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_p"},
			HuggingFaceID:               "CohereForAI/c4ai-command-a-03-2025",
			Description:                 "cohere/command-a on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"cohere/command-a-plus": {
			Ratio:                       nativeRate(0.3),
			CompletionRatio:             5,
			CachedInputRatio:            nativeRate(0.15),
			ContextLength:               192000,
			MaxOutputTokens:             64000,
			InputModalities:             []string{"text", "image"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "max_tokens", "presence_penalty", "reasoning", "response_format", "seed", "stop", "structured_outputs", "temperature", "tools", "top_k", "top_p"},
			Description:                 "cohere/command-a-plus on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"cohere/command-r-08-2024": {
			Ratio:                       nativeRate(0.15),
			CompletionRatio:             4,
			ContextLength:               128000,
			MaxOutputTokens:             4000,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "max_tokens", "presence_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_p"},
			Description:                 "cohere/command-r-08-2024 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"cohere/command-r-plus-08-2024": {
			Ratio:                       nativeRate(2.5),
			CompletionRatio:             4,
			ContextLength:               128000,
			MaxOutputTokens:             4000,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "max_tokens", "presence_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "tool_choice", "tools", "top_k", "top_p"},
			Description:                 "cohere/command-r-plus-08-2024 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"cohere/command-r7b-12-2024": {
			Ratio:                       nativeRate(0.0375),
			CompletionRatio:             4,
			ContextLength:               128000,
			MaxOutputTokens:             4000,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{"frequency_penalty", "max_tokens", "presence_penalty", "response_format", "seed", "stop", "structured_outputs", "temperature", "top_k", "top_p"},
			Description:                 "cohere/command-r7b-12-2024 on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"cohere/north-mini-code:free": {
			Ratio:                       0,
			CompletionRatio:             1,
			ContextLength:               256000,
			MaxOutputTokens:             64000,
			InputModalities:             []string{"text"},
			OutputModalities:            []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"frequency_penalty", "include_reasoning", "max_tokens", "presence_penalty", "reasoning", "seed", "stop", "temperature", "tool_choice", "tools", "top_k", "top_p"},
			HuggingFaceID:               "CohereLabs/North-Mini-Code-1.0",
			Description:                 "cohere/north-mini-code:free on openrouter; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
