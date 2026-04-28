package cohere

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios.
// Model list is derived from the keys of this map, eliminating redundancy.
// Official sources:
// - https://docs.cohere.com/docs/models
// - https://docs.cohere.com/docs/how-does-cohere-pricing-work
// - https://cohere.com/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Current Command Models
	"command-a-03-2025":      {Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4},
	"command-r7b-12-2024":    {Ratio: 0.0375 * ratio.MilliTokensUsd, CompletionRatio: 4},
	"command-r-08-2024":      {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4},
	"command-r-plus-08-2024": {Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4},

	// Command Models
	"command":         {Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2}, // $1/$2 per 1M tokens
	"command-nightly": {Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2}, // $1/$2 per 1M tokens

	// Command Light Models
	"command-light":         {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2}, // $0.3/$0.6 per 1M tokens
	"command-light-nightly": {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2}, // $0.3/$0.6 per 1M tokens

	// Command R Models
	"command-r":      {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3}, // $0.5/$1.5 per 1M tokens
	"command-r-plus": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5},   // $3/$15 per 1M tokens

	// Internet-enabled variants
	"command-internet":               {Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2},   // $1/$2 per 1M tokens
	"command-nightly-internet":       {Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 2},   // $1/$2 per 1M tokens
	"command-light-internet":         {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2}, // $0.3/$0.6 per 1M tokens
	"command-light-nightly-internet": {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2}, // $0.3/$0.6 per 1M tokens
	"command-r-internet":             {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3}, // $0.5/$1.5 per 1M tokens
	"command-r-plus-internet":        {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5},   // $3/$15 per 1M tokens

	// Rerank Models (per-call pricing)
	"rerank-v3.5":              {Ratio: (2.0 / 1000.0) * ratio.QuotaPerUsd},
	"rerank-english-v3.0":      {Ratio: (2.0 / 1000.0) * ratio.QuotaPerUsd},
	"rerank-multilingual-v3.0": {Ratio: (2.0 / 1000.0) * ratio.QuotaPerUsd},
}

// CohereToolingDefaults remains empty because Cohere's official docs and pricing pages publish
// model pricing, but not separate server-side tool invocation fees.
// Sources:
// - https://docs.cohere.com/docs/how-does-cohere-pricing-work
// - https://cohere.com/pricing
var CohereToolingDefaults = adaptor.ChannelToolConfig{}
