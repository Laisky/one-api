package anthropic

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
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

// ModelRatios contains all supported models and their pricing ratios.
//
//   - https://docs.claude.com/en/docs/about-claude/models/overview
//   - https://platform.claude.com/docs/en/about-claude/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Claude 4 Opus Models
	"claude-opus-4-0":          {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
	"claude-opus-4-20250514":   {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
	"claude-opus-4-1":          {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
	"claude-opus-4-1-20250805": {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 75.0 / 15, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
	"claude-opus-4-5":          {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
	"claude-opus-4-5-20251101": {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
	"claude-opus-4-6":          {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
	"claude-opus-4-7":          {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},

	// Claude 4 Sonnet Models
	"claude-sonnet-4-0":          {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-sonnet-4-20250514":   {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-sonnet-4-5":          {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-sonnet-4-5-20250929": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-sonnet-4-6":          {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},

	// Claude 4 Haiku Models
	"claude-haiku-4-5":          {Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.1 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 2 * ratio.MilliTokensUsd},
	"claude-haiku-4-5-20251001": {Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.1 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 2 * ratio.MilliTokensUsd},

	// Claude 3 Opus Models (Deprecated)
	"claude-3-opus-20240229": {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},

	// Claude 3.7 Sonnet Models
	"claude-3-7-sonnet-latest":   {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-3-7-sonnet-20250219": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},

	// Claude 3.5 Sonnet Models
	"claude-3-5-sonnet-latest":   {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-3-5-sonnet-20240620": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-3-5-sonnet-20241022": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
	"claude-3-sonnet-20240229":   {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},

	// Claude 3.5 Haiku Models
	"claude-3-5-haiku-latest":   {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd},
	"claude-3-5-haiku-20241022": {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd},

	// Claude 3 Haiku Models
	"claude-3-haiku-20240307": {Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.03 * ratio.MilliTokensUsd, CacheWrite5mRatio: 0.30 * ratio.MilliTokensUsd, CacheWrite1hRatio: 0.5 * ratio.MilliTokensUsd},

	// Legacy Models
	"claude-2.1":         {Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0, CachedInputRatio: 0.8 * ratio.MilliTokensUsd, CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd},
	"claude-2.0":         {Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0, CachedInputRatio: 0.8 * ratio.MilliTokensUsd, CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd},
	"claude-instant-1.2": {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 3.0, CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd},
}

const anthropicWebSearchUsdPerCall = 10.0 / 1000.0

// AnthropicToolingDefaults represents Anthropic's published built-in tool pricing (2026-04-16).
// Source: https://r.jina.ai/https://docs.claude.com/en/docs/build-with-claude/tool-use/web-search-tool
var AnthropicToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		ToolTypeWebSearch: {UsdPerCall: anthropicWebSearchUsdPerCall},
	},
}
