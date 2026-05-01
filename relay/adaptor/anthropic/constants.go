package anthropic

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

const (
	// AnthropicVersionDefault is Anthropic's default API version header.
	AnthropicVersionDefault = "2023-06-01"
	// AnthropicBetaMessages is the baseline messages beta header used by Anthropic adaptor.
	AnthropicBetaMessages = "messages-2023-12-15"
	// AnthropicBetaAdvancedToolUse gates Anthropic's advanced tool-use features, including Tool Search.
	AnthropicBetaAdvancedToolUse = "advanced-tool-use-2025-11-20"

	// ToolTypeWebSearch is the canonical web-search built-in identifier.
	ToolTypeWebSearch = "web_search"
	// ToolTypeWebSearchPreview is an alias used by some OpenAI-compatible providers.
	ToolTypeWebSearchPreview = "web_search_preview"

	// ToolTypeToolSearchRegex is Anthropic Tool Search regex tool identifier.
	ToolTypeToolSearchRegex = "tool_search_tool_regex"
	// ToolTypeToolSearchBM25 is Anthropic Tool Search BM25 tool identifier.
	ToolTypeToolSearchBM25 = "tool_search_tool_bm25"

	// ToolTypeToolSearchRegexPrefix matches versioned Anthropic regex Tool Search identifiers.
	ToolTypeToolSearchRegexPrefix = "tool_search_tool_regex_"
	// ToolTypeToolSearchBM25Prefix matches versioned Anthropic BM25 Tool Search identifiers.
	ToolTypeToolSearchBM25Prefix = "tool_search_tool_bm25_"
)

// Shared metadata helpers for Anthropic Claude models. These slices are reused
// across many ModelRatios entries to keep the table compact and consistent.
var (
	// claudeVisionInputs lists the input modalities for Claude 3+ vision-capable models.
	claudeVisionInputs = []string{"text", "image", "file"}
	// claudeTextInputs lists the input modalities for legacy text-only Claude models.
	claudeTextInputs = []string{"text"}
	// claudeTextOutputs lists the output modalities for all Claude chat models.
	claudeTextOutputs = []string{"text"}

	// claudeFeaturesWithReasoning advertises the capability set for Claude 4+ models that
	// support extended/adaptive thinking in addition to tools and structured output.
	claudeFeaturesWithReasoning = []string{"tools", "json_mode", "structured_outputs", "web_search", "reasoning"}
	// claudeFeaturesNoReasoning advertises the capability set for Claude 3.x models that
	// support tools and structured output but not extended thinking.
	claudeFeaturesNoReasoning = []string{"tools", "json_mode", "structured_outputs", "web_search"}
	// claudeLegacyFeatures advertises the limited capability set for Claude 2.x / instant.
	claudeLegacyFeatures = []string{}

	// claudeSamplingParams lists the sampling parameters Claude chat completions accept.
	claudeSamplingParams = []string{"temperature", "top_p", "top_k", "stop", "max_tokens"}
)

// ModelRatios contains all supported models and their pricing ratios.
//
//   - https://docs.claude.com/en/docs/about-claude/models/overview
//   - https://platform.claude.com/docs/en/about-claude/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Claude 4 Opus Models
	"claude-opus-4-0": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Opus 4 (alias for claude-opus-4-20250514).",
	},
	"claude-opus-4-20250514": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Opus 4 frontier model with extended thinking (deprecated; retire 2026-06-15).",
	},
	"claude-opus-4-1": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Opus 4.1 (alias for claude-opus-4-1-20250805).",
	},
	"claude-opus-4-1-20250805": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 75.0 / 15,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 32000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Opus 4.1 frontier reasoning model with extended thinking.",
	},
	"claude-opus-4-5": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Opus 4.5 (alias for claude-opus-4-5-20251101).",
	},
	"claude-opus-4-5-20251101": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Opus 4.5 frontier model with extended thinking.",
	},
	"claude-opus-4-6": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Opus 4.6 with 1M-token context and extended thinking.",
	},
	"claude-opus-4-7": {
		Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5,
		CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 128000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Opus 4.7 most capable Anthropic model with 1M-token context and adaptive thinking.",
	},

	// Claude 4 Sonnet Models
	"claude-sonnet-4-0": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Sonnet 4 (alias for claude-sonnet-4-20250514).",
	},
	"claude-sonnet-4-20250514": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Sonnet 4 with extended thinking (deprecated; retire 2026-06-15).",
	},
	"claude-sonnet-4-5": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Sonnet 4.5 (alias for claude-sonnet-4-5-20250929).",
	},
	"claude-sonnet-4-5-20250929": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Sonnet 4.5 balanced flagship with extended thinking.",
	},
	"claude-sonnet-4-6": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 1000000, MaxOutputTokens: 64000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Sonnet 4.6 with 1M-token context, extended and adaptive thinking.",
	},

	// Claude 4 Haiku Models
	"claude-haiku-4-5": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.1 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 2 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Haiku 4.5 (alias for claude-haiku-4-5-20251001).",
	},
	"claude-haiku-4-5-20251001": {
		Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.1 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 2 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 64000,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Haiku 4.5 fastest near-frontier model with extended thinking.",
	},

	// Claude 3 Opus Models (Deprecated)
	"claude-3-opus-20240229": {
		Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesNoReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3 Opus legacy high-intelligence model (deprecated).",
	},

	// Claude 3.7 Sonnet Models
	"claude-3-7-sonnet-latest": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3.7 Sonnet alias tracking the latest 3.7 snapshot.",
	},
	"claude-3-7-sonnet-20250219": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesWithReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3.7 Sonnet first hybrid reasoning model with extended thinking.",
	},

	// Claude 3.5 Sonnet Models
	"claude-3-5-sonnet-latest": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesNoReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3.5 Sonnet alias tracking the latest 3.5 Sonnet snapshot.",
	},
	"claude-3-5-sonnet-20240620": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesNoReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3.5 Sonnet original release (June 2024).",
	},
	"claude-3-5-sonnet-20241022": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesNoReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3.5 Sonnet v2 (October 2024) with computer-use beta.",
	},
	"claude-3-sonnet-20240229": {
		Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesNoReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3 Sonnet legacy mid-tier model (deprecated).",
	},

	// Claude 3.5 Haiku Models
	"claude-3-5-haiku-latest": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesNoReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3.5 Haiku alias tracking the latest 3.5 Haiku snapshot.",
	},
	"claude-3-5-haiku-20241022": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 8192,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesNoReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3.5 Haiku fast, cost-efficient model.",
	},

	// Claude 3 Haiku Models
	"claude-3-haiku-20240307": {
		Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 5.0,
		CachedInputRatio: 0.03 * ratio.MilliTokensUsd, CacheWrite5mRatio: 0.30 * ratio.MilliTokensUsd, CacheWrite1hRatio: 0.5 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: claudeVisionInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeFeaturesNoReasoning, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 3 Haiku legacy fast, low-cost model with vision support.",
	},

	// Legacy Models
	"claude-2.1": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0,
		CachedInputRatio: 0.8 * ratio.MilliTokensUsd, CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 200000, MaxOutputTokens: 4096,
		InputModalities: claudeTextInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeLegacyFeatures, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 2.1 legacy text-only model (no vision, no tools).",
	},
	"claude-2.0": {
		Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0,
		CachedInputRatio: 0.8 * ratio.MilliTokensUsd, CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd,
		ContextLength: 100000, MaxOutputTokens: 4096,
		InputModalities: claudeTextInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeLegacyFeatures, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude 2.0 legacy text-only model (no vision, no tools).",
	},
	"claude-instant-1.2": {
		Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 3.0,
		CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd,
		ContextLength: 100000, MaxOutputTokens: 4096,
		InputModalities: claudeTextInputs, OutputModalities: claudeTextOutputs,
		SupportedFeatures: claudeLegacyFeatures, SupportedSamplingParameters: claudeSamplingParams,
		Description: "Claude Instant 1.2 legacy fast text-only model (no vision, no tools).",
	},
}

const anthropicWebSearchUsdPerCall = 10.0 / 1000.0

// AnthropicToolingDefaults represents Anthropic's published built-in tool pricing (2026-04-16).
// Source: https://r.jina.ai/https://docs.claude.com/en/docs/build-with-claude/tool-use/web-search-tool
var AnthropicToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		ToolTypeWebSearch: {UsdPerCall: anthropicWebSearchUsdPerCall},
	},
}
