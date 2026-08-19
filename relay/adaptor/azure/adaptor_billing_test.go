package azure

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
)

// TestPricingCatalogsAreDisjoint is the collision guard for GetDefaultModelPricing.
// The merge unions the OpenAI and Anthropic catalogs with last-write-wins; if a
// model id ever appeared in both, one family's price would silently shadow the
// other. This test fails the moment such an overlap is introduced.
func TestPricingCatalogsAreDisjoint(t *testing.T) {
	t.Parallel()

	for name := range openai.ModelRatios {
		_, dup := anthropic.ModelRatios[name]
		require.Falsef(t, dup,
			"model %q exists in BOTH openai and anthropic catalogs; the Azure pricing merge would silently pick one price", name)
	}

	a := &Adaptor{}
	merged := a.GetDefaultModelPricing()
	require.Equal(t, len(openai.ModelRatios)+len(anthropic.ModelRatios), len(merged),
		"merged pricing size must equal the sum of both catalogs — a smaller size means a key collision dropped an entry")
}

// TestEveryAdvertisedModelBillsAtItsFamilyRate quantifies billing correctness for
// the ENTIRE advertised Azure catalog: every model resolves to a positive ratio,
// Claude models bill at the exact Anthropic rate, and everything else at the exact
// OpenAI rate. If Azure ever advertised a model it prices wrong, this catches it.
func TestEveryAdvertisedModelBillsAtItsFamilyRate(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	oai := &openai.Adaptor{}
	ant := &anthropic.Adaptor{}

	for _, name := range a.GetModelList() {
		modelRatio := a.GetModelRatio(name)
		completionRatio := a.GetCompletionRatio(name)

		if meta.IsClaudeModelName(name) {
			// Every Foundry Claude model is token-priced, so its ratio must be positive
			// AND must match the Anthropic catalog exactly.
			require.Greaterf(t, modelRatio, 0.0, "Claude model %q must have a positive model ratio", name)
			require.Equalf(t, ant.GetModelRatio(name), modelRatio,
				"Claude model %q must bill at the Anthropic model ratio", name)
			require.Equalf(t, ant.GetCompletionRatio(name), completionRatio,
				"Claude model %q must bill at the Anthropic completion ratio", name)
		} else {
			// GPT-on-Azure must bill IDENTICALLY to the OpenAI adaptor. We assert parity
			// (not positivity) because the OpenAI catalog legitimately prices some models
			// at token-ratio 0 (e.g. realtime models billed via per-second audio config).
			require.Equalf(t, oai.GetModelRatio(name), modelRatio,
				"OpenAI model %q must bill at the OpenAI model ratio", name)
			require.Equalf(t, oai.GetCompletionRatio(name), completionRatio,
				"OpenAI model %q must bill at the OpenAI completion ratio", name)
		}
	}
}

// TestFoundryClaudeModelsHaveRealAnthropicPricing proves none of the curated
// Foundry Claude models silently fall back to a default price: each must be present
// in the merged pricing map AND match the Anthropic catalog exactly (input +
// completion ratios). This is the guarantee behind "Claude-on-Azure bills at
// Anthropic rates".
func TestFoundryClaudeModelsHaveRealAnthropicPricing(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	merged := a.GetDefaultModelPricing()

	for _, name := range FoundryClaudeModels {
		require.Truef(t, meta.IsClaudeModelName(name),
			"FoundryClaudeModels entry %q must be recognized as a Claude model by the routing predicate", name)

		antCfg, inAnthropic := anthropic.ModelRatios[name]
		require.Truef(t, inAnthropic,
			"Foundry Claude model %q must have a real anthropic.ModelRatios entry (else it bills at the 3x default fallback)", name)

		mergedCfg, inMerged := merged[name]
		require.Truef(t, inMerged, "Foundry Claude model %q must be present in the merged Azure pricing map", name)
		require.Greaterf(t, mergedCfg.Ratio, 0.0, "Foundry Claude model %q must have a positive input ratio", name)
		require.Equalf(t, antCfg.Ratio, mergedCfg.Ratio,
			"Foundry Claude model %q input ratio must match the Anthropic catalog", name)
		require.Equalf(t, antCfg.CompletionRatio, mergedCfg.CompletionRatio,
			"Foundry Claude model %q completion ratio must match the Anthropic catalog", name)
	}
}
