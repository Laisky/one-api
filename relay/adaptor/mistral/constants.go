package mistral

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Official sources:
// - https://docs.mistral.ai/models/overview
// - https://docs.mistral.ai/resources/changelogs
// - model cards under https://docs.mistral.ai/models/model-cards/
var ModelRatios = map[string]adaptor.ModelConfig{
	"mistral-medium-latest":   {Ratio: 0.4 * ratio.MilliTokensUsd, CompletionRatio: 5.0},  // $0.4 input, $2 output
	"mistral-medium-2508":     {Ratio: 0.4 * ratio.MilliTokensUsd, CompletionRatio: 5.0},  // $0.4 input, $2 output
	"magistral-medium-latest": {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 2.5},  // $2 input, $5 output
	"magistral-medium-2509":   {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 2.5},  // $2 input, $5 output
	"devstral-2512":           {Ratio: 0.4 * ratio.MilliTokensUsd, CompletionRatio: 5.0},  // $0.4 input, $2 output
	"devstral-medium-2507":    {Ratio: 0.4 * ratio.MilliTokensUsd, CompletionRatio: 5.0},  // $0.4 input, $2 output
	"codestral-latest":        {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $0.3 input, $0.9 output
	"codestral-2508":          {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $0.3 input, $0.9 output
	"mistral-large-latest":    {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $0.5 input, $1.5 output
	"mistral-large-2512":      {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $0.5 input, $1.5 output
	"pixtral-large-latest":    {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $2 input, $6 output
	"mistral-saba-latest":     {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $0.2 input, $0.6 output
	"mistral-small-latest":    {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4.0}, // $0.15 input, $0.6 output
	"magistral-small-latest":  {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $0.5 input, $1.5 output
	"magistral-small-2509":    {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $0.5 input, $1.5 output
	"devstral-small-2507":     {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $0.1 input, $0.3 output
	"pixtral-12b":             {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // $0.15 input, $0.15 output
	"open-mistral-7b":         {Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // $0.25 input, $0.25 output
	"open-mixtral-8x7b":       {Ratio: 0.7 * ratio.MilliTokensUsd, CompletionRatio: 1.0},  // $0.7 input, $0.7 output
	"open-mixtral-8x22b":      {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 3.0},  // $2 input, $6 output
	"ministral-14b-2512":      {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1.0},  // $0.2 input, $0.2 output
	"ministral-8b-latest":     {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // $0.15 input, $0.15 output
	"ministral-8b-2512":       {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // $0.15 input, $0.15 output
	"ministral-3b-latest":     {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1.0},  // $0.1 input, $0.1 output
	"ministral-3b-2512":       {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1.0},  // $0.1 input, $0.1 output

	// Embedding Models
	"mistral-embed":        {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1.0},  // $0.1 input only
	"codestral-embed-2505": {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // $0.15 input only
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MistralToolingDefaults preserves legacy tool defaults.
// Mistral's official model overview, changelog, and accessible model-card pages do not
// currently publish equivalent per-tool invocation pricing, so these values remain unchanged.
var MistralToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"code_execution":     {UsdPerCall: 0.01}, // Connectors and code tools: $0.01 per API call
		"document_library":   {UsdPerCall: 0.01}, // Document library billed under connector rate
		"image_generation":   {UsdPerCall: 0.10}, // $100 per 1K images
		"web_search":         {UsdPerCall: 0.03}, // Web search / knowledge plugins: $30 per 1K queries
		"web_search_premium": {UsdPerCall: 0.03},
	},
}
