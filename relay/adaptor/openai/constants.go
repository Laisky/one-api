package openai

import "github.com/Laisky/one-api/relay/adaptor"

// OpenAIToolingDefaults enumerates OpenAI's built-in tool whitelist and pricing (retrieved 2026-07-31).
// Source: https://developers.openai.com/api/docs/pricing#tools
var OpenAIToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"code_interpreter":                 {UsdPerCall: 0.03},   // $0.03 per default 1 GB container session
		"file_search":                      {UsdPerCall: 0.0025}, // $2.50 per 1K tool calls
		"web_search":                       {UsdPerCall: 0.01},   // $10 per 1K tool calls
		"web_search_preview_reasoning":     {UsdPerCall: 0.01},   // Preview tier for reasoning models, $10 per 1K tool calls
		"web_search_preview_non_reasoning": {UsdPerCall: 0.025},  // Preview tier for non-reasoning models, $25 per 1K tool calls
	},
}
