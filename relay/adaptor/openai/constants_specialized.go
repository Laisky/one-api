package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// specializedModelRatios captures pricing and metadata for OpenAI's specialized
// chat models: codex, web-search-preview, computer-use, and moderation.
// Sources:
//   - https://platform.openai.com/docs/models/codex-mini-latest
//   - https://platform.openai.com/docs/models/gpt-4o-search-preview
//   - https://platform.openai.com/docs/models/computer-use-preview
//   - https://platform.openai.com/docs/models/omni-moderation-latest
var specializedModelRatios = map[string]adaptor.ModelConfig{
	// Codex Models
	"codex-mini-latest": {
		Ratio:                       1.5 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		CachedInputRatio:            0.375 * ratio.MilliTokensUsd,
		ContextLength:               200000,
		MaxOutputTokens:             100000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning", "tools"},
		SupportedSamplingParameters: reasoningSamplingParameters(),
		Description:                 "Codex mini latest: lightweight Codex agent reasoning model.",
	},

	// Search-Preview Models (web-search-augmented chat completions)
	"gpt-4o-mini-search-preview": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "web_search"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4o mini Search preview: chat-completions endpoint with built-in web search.",
	},
	"gpt-4o-mini-search-preview-2025-03-11": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "web_search"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4o mini Search preview snapshot from 2025-03-11.",
	},
	"gpt-4o-search-preview": {
		Ratio:                       2.5 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "web_search"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4o Search preview: chat-completions endpoint with built-in web search.",
	},
	"gpt-4o-search-preview-2025-03-11": {
		Ratio:                       2.5 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		ContextLength:               128000,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "web_search"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4o Search preview snapshot from 2025-03-11.",
	},

	// Computer Use Models
	"computer-use-preview": {
		Ratio:                       3.0 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		ContextLength:               8192,
		MaxOutputTokens:             1024,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "Computer Use preview: agent model that emits computer-control actions.",
	},
	"computer-use-preview-2025-03-11": {
		Ratio:                       3.0 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		ContextLength:               8192,
		MaxOutputTokens:             1024,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "Computer Use preview snapshot from 2025-03-11.",
	},

	// Moderation Models (free; classifier endpoints)
	"text-moderation-latest": {
		Ratio:            0.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    32768,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "Text Moderation (latest alias): free safety classifier.",
	},
	"text-moderation-stable": {
		Ratio:            0.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    32768,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "Text Moderation (stable alias): free safety classifier.",
	},
	"text-moderation-007": {
		Ratio:            0.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    32768,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "Text Moderation v007: pinned safety classifier.",
	},
	"omni-moderation-latest": {
		Ratio:            0.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    32768,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
		Description:      "Omni Moderation (latest alias): multimodal safety classifier.",
	},
	"omni-moderation-2024-09-26": {
		Ratio:            0.0 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		ContextLength:    32768,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
		Description:      "Omni Moderation snapshot from 2024-09-26.",
	},
}
