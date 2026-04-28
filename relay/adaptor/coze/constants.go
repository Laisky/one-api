package coze

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains a conservative Coze compatibility placeholder.
// Model list is derived from the keys of this map, eliminating redundancy.
// Coze's public premium page publishes subscription credits and per-call credit costs, not canonical per-token USD pricing,
// so this file intentionally stays stable rather than inferring token economics from plan bundles.
var ModelRatios = map[string]adaptor.ModelConfig{
	// Coze models - retained as a compatibility alias because the public pricing is credit-based rather than token-based.
	"coze-chat": {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// CozeToolingDefaults notes that Coze's public pricing page lists subscription tiers but no per-tool metering (retrieved 2026-04-28).
// Source: https://www.coze.com/premium
var CozeToolingDefaults = adaptor.ChannelToolConfig{}

const (
	PersonalAccessToken = "personal_access_token"
	OAuthJWT            = "oauth_jwt"
)
