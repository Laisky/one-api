package vertexai

import "github.com/Laisky/one-api/relay/adaptor"

// VertexAIToolingDefaults captures Vertex AI's published tooling charges (retrieved 2026-05-18).
// Source: https://cloud.google.com/vertex-ai/generative-ai/pricing
var VertexAIToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"google_search_grounding":  {UsdPerCall: 0.035},
		"web_grounding_enterprise": {UsdPerCall: 0.045},
		"grounding_with_your_data": {UsdPerCall: 0.0025},
		"google_maps_grounding":    {UsdPerCall: 0.025},
		"claude_web_search":        {UsdPerCall: 0.01},
	},
}

// VertexAIClaudeToolingDefaults extends the Vertex defaults for Claude partner
// models (verified 2026-10-05). Vertex offers only Anthropic's basic web search
// ("Web Search Request $10 per 1000 searches"); web fetch, code execution and
// the MCP connector are not offered on Vertex and therefore stay unpriced so
// they fail closed. Tool Search is unmetered and admitted by the shared policy.
// Sources: https://cloud.google.com/vertex-ai/generative-ai/pricing and
// https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
var VertexAIClaudeToolingDefaults = func() adaptor.ChannelToolConfig {
	pricing := make(map[string]adaptor.ToolPricingConfig, len(VertexAIToolingDefaults.Pricing)+1)
	for name, cfg := range VertexAIToolingDefaults.Pricing {
		pricing[name] = cfg
	}
	pricing["web_search"] = adaptor.ToolPricingConfig{UsdPerCall: 0.01}
	return adaptor.ChannelToolConfig{Pricing: pricing}
}()
