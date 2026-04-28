package xai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	ratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Official sources:
// - https://docs.x.ai/developers/models
// - https://docs.x.ai/developers/model-capabilities/text/multi-agent
// - https://docs.x.ai/developers/tools/web-search
var ModelRatios = map[string]adaptor.ModelConfig{
	// Grok Models
	//
	// Note: Some prices are the same because they are aliases or stable/snapshot pairs.
	"grok-code-fast-1":             {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 1.5 / 0.2, CachedInputRatio: 0.02 * ratio.MilliTokensUsd}, // $0.20 input, $0.02 cached input, $1.50 output
	"grok-4-0709":                  {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 15.0 / 3.0, CachedInputRatio: 0.75 * ratio.MilliTokensUsd},
	"grok-4.20":                    {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd},
	"grok-4.20-reasoning":          {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd},
	"grok-4.20-non-reasoning":      {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd},
	"grok-4.20-multi-agent":        {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd},
	"grok-4.20-0309-reasoning":     {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd},
	"grok-4.20-0309-non-reasoning": {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd},
	"grok-4.20-multi-agent-0309":   {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 6.0 / 2.0, CachedInputRatio: 0.2 * ratio.MilliTokensUsd},
	"grok-4-1-fast":                {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.2, CachedInputRatio: 0.05 * ratio.MilliTokensUsd},
	"grok-4-1-fast-reasoning":      {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.2, CachedInputRatio: 0.05 * ratio.MilliTokensUsd},
	"grok-4-1-fast-non-reasoning":  {Ratio: 0.2 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.2, CachedInputRatio: 0.05 * ratio.MilliTokensUsd},
	"grok-3":                       {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0 / 3.0, CachedInputRatio: 0.75 * ratio.MilliTokensUsd},
	"grok-3-mini":                  {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.3, CachedInputRatio: 0.075 * ratio.MilliTokensUsd},
	// "grok-3-fast":               {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.75 * ratio.MilliTokensUsd},        // $3.00 input, $0.75 cached input, $15.00 output
	// "grok-3-mini-fast":          {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.3, CachedInputRatio: 0.075 * ratio.MilliTokensUsd}, // $0.30 input, $0.075 cached input, $0.50 output
	"grok-2-vision-1212": {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 10.0 / 2.0}, // $2.00 input, $10.00 output
	// "grok-2-1212":        {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0},        // $2.00 input, $10.00 output

	// Image generation model (no per-token charge)
	"grok-imagine-image": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.02,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
			},
		},
	},
	"grok-imagine-image-pro": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.07,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
			},
		},
	},
	"grok-2-image-1212": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.07,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
			},
		},
	}, // $0.07 per image
	"grok-2-image": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.07,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        10,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
			},
		},
	}, // $0.07 per image

	"grok-imagine-video": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd: 0.05,
		},
	}, // $0.05 per second

	// Legacy aliases for backward compatibility
	// "grok-beta":        {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0}, // Updated to match grok-2-1212
	// "grok-2":           {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0}, // Updated to match grok-2-1212
	// "grok-2-latest":    {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0}, // Updated to match grok-2-1212
	// "grok-vision-beta": {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 5.0}, // Updated to match grok-2-vision-1212
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// XAIToolingDefaults captures xAI's published tool invocation fees.
// Source: https://docs.x.ai/developers/models
var XAIToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"web_search":         {UsdPerCall: 0.005},  // $5 / 1k calls
		"x_search":           {UsdPerCall: 0.005},  // $5 / 1k calls
		"code_execution":     {UsdPerCall: 0.005},  // $5 / 1k calls
		"code_interpreter":   {UsdPerCall: 0.005},  // alias of code_execution in Responses API
		"attachment_search":  {UsdPerCall: 0.01},   // $10 / 1k calls (File Attachments)
		"collections_search": {UsdPerCall: 0.0025}, // $2.50 / 1k calls
		"file_search":        {UsdPerCall: 0.0025}, // alias of collections_search in Responses API
	},
}
