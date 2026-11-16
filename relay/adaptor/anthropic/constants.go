package anthropic

import "github.com/Laisky/one-api/relay/adaptor"

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
	// ToolTypeWebFetch is the canonical server-side web fetch capability.
	ToolTypeWebFetch = "web_fetch"
	// ToolTypeCodeExecution is the canonical server-side code execution capability.
	ToolTypeCodeExecution = "code_execution"
	// ToolTypeToolSearch is the canonical, unmetered Tool Search capability.
	ToolTypeToolSearch = "tool_search"
	// ToolTypeMCPConnector is the canonical provider-side MCP connector capability.
	ToolTypeMCPConnector = "mcp_connector"

	// ToolTypeToolSearchRegex is Anthropic Tool Search regex tool identifier.
	ToolTypeToolSearchRegex = "tool_search_tool_regex"
	// ToolTypeToolSearchBM25 is Anthropic Tool Search BM25 tool identifier.
	ToolTypeToolSearchBM25 = "tool_search_tool_bm25"

	// ToolTypeToolSearchRegexPrefix matches versioned Anthropic regex Tool Search identifiers.
	ToolTypeToolSearchRegexPrefix = "tool_search_tool_regex_"
	// ToolTypeToolSearchBM25Prefix matches versioned Anthropic BM25 Tool Search identifiers.
	ToolTypeToolSearchBM25Prefix = "tool_search_tool_bm25_"
)

const anthropicWebSearchUsdPerCall = 10.0 / 1000.0

// AnthropicToolingDefaults represents Anthropic's published server-tool pricing (verified 2026-10-05).
//
//   - web_search: $10 per 1,000 searches; failed searches are not billed.
//     https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool
//   - web_fetch: "no additional charges beyond standard token costs", so it is
//     priced explicitly at zero instead of being left unpriced.
//     https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-fetch-tool
//   - tool_search: not metered as a server tool; loaded definitions bill as input tokens.
//     https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool
//
// Code execution is intentionally absent: it is billed per container-hour
// ($0.05/hour after a monthly free allowance, 5-minute minimum) and has no
// per-call tariff the gateway can meter, so it fails closed until an operator
// configures a channel price. The MCP connector (mcp_servers) is likewise absent
// because it lets the provider reach caller-chosen remote servers.
// https://platform.claude.com/docs/en/about-claude/pricing
var AnthropicToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		ToolTypeWebSearch:  {UsdPerCall: anthropicWebSearchUsdPerCall},
		ToolTypeWebFetch:   {},
		ToolTypeToolSearch: {},
	},
}
