package vertexai

import "github.com/Laisky/one-api/relay/adaptor"

// VertexAIToolingDefaults captures Vertex AI's published tooling charges (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://cloud.google.com/vertex-ai/generative-ai/pricing
var VertexAIToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"google_search_grounding":  {UsdPerCall: 0.035},
		"web_grounding_enterprise": {UsdPerCall: 0.045},
		"grounding_with_your_data": {UsdPerCall: 0.0025},
		"google_maps_grounding":    {UsdPerCall: 0.025},
		"claude_web_search":        {UsdPerCall: 0.01},
	},
}
