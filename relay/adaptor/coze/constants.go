package coze

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Coze models are typically free or very low cost for basic usage
var ModelRatios = map[string]adaptor.ModelConfig{
	// Coze models - estimated pricing as Coze doesn't publish detailed pricing
	"coze-chat": {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// CozeToolingDefaults notes that Coze's public pricing page lists subscription tiers but no per-tool metering (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://www.coze.com/premium
var CozeToolingDefaults = adaptor.ChannelToolConfig{}

const (
	PersonalAccessToken = "personal_access_token"
	OAuthJWT            = "oauth_jwt"
)
