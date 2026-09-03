package deepl

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// deeplUsdPerMillionCharacters is DeepL's published usage rate, $27.50 per
// 1,000,000 characters (API Growth overage and Enterprise usage alike; the fixed
// monthly plan fee is not a per-request charge and is not modelled here).
// Retrieved 2026-09-02 from DeepL's own pricing backend behind
// https://www.deepl.com/en/pro#developer. DeepL publishes no price split between
// latency_optimized and quality_optimized model types.
const deeplUsdPerMillionCharacters = 27.5

// ModelRatios prices DeepL per SOURCE CHARACTER, which is the unit this adaptor
// actually meters: DoResponse reports `len(a.promptText)` as PromptTokens, i.e.
// the character count of the source text, so the framework's per-token ratio is a
// per-character ratio here and DeepL's published per-character rate applies
// directly.
//
// Without these entries the adaptor advertised three models it could not price
// and every translation billed at the 2.5 USD/1M fallback in
// adaptor.DefaultPricingMethods — about 11x under DeepL's own rate.
//
// CompletionRatio is 1 for completeness only; DoResponse reports no completion
// tokens because DeepL bills source characters, not output.
var ModelRatios = map[string]adaptor.ModelConfig{
	"deepl-zh": {
		Ratio:           deeplUsdPerMillionCharacters * ratio.MilliTokensUsd,
		CompletionRatio: 1,
		Description:     "DeepL translation into Chinese, billed per source character.",
	},
	"deepl-en": {
		Ratio:           deeplUsdPerMillionCharacters * ratio.MilliTokensUsd,
		CompletionRatio: 1,
		Description:     "DeepL translation into English, billed per source character.",
	},
	"deepl-ja": {
		Ratio:           deeplUsdPerMillionCharacters * ratio.MilliTokensUsd,
		CompletionRatio: 1,
		Description:     "DeepL translation into Japanese, billed per source character.",
	},
}

// ModelList retains compatibility aliases for common DeepL routes.
// DeepL's official API exposes language pairs and model_type choices rather than canonical model IDs,
// so these entries remain intentionally curated.

var ModelList = []string{
	"deepl-zh",
	"deepl-en",
	"deepl-ja",
}

// DeepLToolingDefaults captures that DeepL's translation API does not publish per-call tooling charges (retrieved 2026-04-28).
// Source: https://developers.deepl.com/docs/api-reference
var DeepLToolingDefaults = adaptor.ChannelToolConfig{}
