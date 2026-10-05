package aws

import "github.com/Laisky/one-api/relay/adaptor"

// AWSToolingDefaults captures Amazon Bedrock AgentCore pricing for common server-side tools (retrieved 2026-04-28).
// Source: https://aws.amazon.com/bedrock/agentcore/pricing/
//
// For Claude on Bedrock (verified 2026-10-05), Anthropic's Tool Search works via
// InvokeModel and is unmetered, so it is priced explicitly at zero. Anthropic's
// web search, web fetch, code execution and MCP connector server tools are not
// supported on Bedrock and stay unpriced so the gateway rejects them before
// dispatch. Source: https://platform.claude.com/docs/en/build-with-claude/claude-in-amazon-bedrock
var AWSToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"tool_search":                         {},
		"agentcore_search_api":                {UsdPerCall: 0.000025},
		"agentcore_invoke_tool":               {UsdPerCall: 0.000005},
		"agentcore_identity_token":            {UsdPerCall: 0.00001},
		"agentcore_memory_short_term":         {UsdPerCall: 0.00025},
		"agentcore_memory_long_term_store":    {UsdPerCall: 0.00075},
		"agentcore_memory_long_term_retrieve": {UsdPerCall: 0.0005},
	},
}
