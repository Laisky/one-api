package stepfun

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on StepFun pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// StepFun Models - estimated pricing
	"step-1-8k":      {Ratio: 1.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"step-1-32k":     {Ratio: 2.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"step-1-128k":    {Ratio: 4.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"step-1-256k":    {Ratio: 8.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"step-1-flash":   {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"step-2-16k":     {Ratio: 3.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"step-1v-8k":     {Ratio: 1.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"step-1v-32k":    {Ratio: 3.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"step-1x-medium": {Ratio: 2.0 * ratio.MilliTokensRmb, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// StepFunToolingDefaults notes that StepFun publishes model pricing only; no tool-specific fees are documented (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://stepfun.ai/
var StepFunToolingDefaults = adaptor.ChannelToolConfig{}
