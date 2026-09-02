package deepl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestDeepLIsPricedPerSourceCharacter pins both halves of DeepL billing: that the
// adaptor prices the models it advertises at all, and that the rate is expressed
// in DeepL's own unit.
//
// DeepL is metered in characters, not tokens — DoResponse reports
// len(a.promptText) as PromptTokens — so the framework's per-token ratio is a
// per-character ratio here. The adaptor previously embedded DefaultPricingMethods
// without publishing any table, so every translation billed at the 2.5 USD/1M
// fallback, roughly 11x below DeepL's published $27.50 per 1M characters.
func TestDeepLIsPricedPerSourceCharacter(t *testing.T) {
	adaptor := &Adaptor{}
	pricing := adaptor.GetDefaultModelPricing()
	require.NotEmpty(t, pricing)

	for _, modelName := range ModelList {
		t.Run(modelName, func(t *testing.T) {
			cfg, ok := pricing[modelName]
			require.Truef(t, ok, "%s is advertised in ModelList but carries no price", modelName)

			usdPerMillionCharacters := cfg.Ratio / ratio.MilliTokensUsd
			require.InDelta(t, deeplUsdPerMillionCharacters, usdPerMillionCharacters, 1e-9,
				"the ratio must express DeepL's published per-character rate")
			require.Equal(t, cfg.Ratio, adaptor.GetModelRatio(modelName))
			require.Equal(t, cfg.CompletionRatio, adaptor.GetCompletionRatio(modelName))
		})
	}
}

// TestDeepLUnknownModelFallsBackToFrameworkDefault keeps the fallback reachable
// for ids the table does not cover.
func TestDeepLUnknownModelFallsBackToFrameworkDefault(t *testing.T) {
	adaptor := &Adaptor{}
	require.InDelta(t, 2.5*ratio.MilliTokensUsd, adaptor.GetModelRatio("deepl-unknown"), 1e-9)
}
