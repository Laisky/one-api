package tooling

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// perModelToolingStub implements ToolingDefaultsForModelProvider so we can assert
// that buildToolPolicy threads the request model name into per-model resolution.
type perModelToolingStub struct {
	adaptor.Adaptor
	gotModel string
}

func (s *perModelToolingStub) DefaultToolingConfigForModel(model string) adaptor.ChannelToolConfig {
	s.gotModel = model
	return adaptor.ChannelToolConfig{
		Pricing: map[string]adaptor.ToolPricingConfig{
			"web_search": {UsdPerCall: 0.014},
		},
	}
}

// TestBuildToolPolicy_PrefersPerModelToolingDefaults verifies the policy builder
// prefers a provider's per-model tooling defaults and passes the model name through.
func TestBuildToolPolicy_PrefersPerModelToolingDefaults(t *testing.T) {
	stub := &perModelToolingStub{}
	policy := buildToolPolicy(nil, stub, "gemini-3.1-pro-preview")
	require.Equal(t, "gemini-3.1-pro-preview", stub.gotModel,
		"model name must be threaded to the per-model tooling resolver")
	require.Equal(t, int64(math.Ceil(0.014*float64(ratio.QuotaPerUsd))), policy.pricing["web_search"])
}

// TestGeminiWebSearchPricingIsPerModel locks in the billing fix: Gemini 3.x grounded
// web search is $14/1K queries while Gemini 2.5 and earlier remain $35/1K.
func TestGeminiWebSearchPricingIsPerModel(t *testing.T) {
	a := &gemini.Adaptor{}

	policy3x := buildToolPolicy(nil, a, "gemini-3.1-pro-preview")
	policy25 := buildToolPolicy(nil, a, "gemini-2.5-flash")

	want3x := int64(math.Ceil(14.0 / 1000.0 * float64(ratio.QuotaPerUsd)))
	want25 := int64(math.Ceil(35.0 / 1000.0 * float64(ratio.QuotaPerUsd)))

	require.Equal(t, want3x, policy3x.pricing["web_search"], "Gemini 3.x web_search should bill $14/1K queries")
	require.Equal(t, want25, policy25.pricing["web_search"], "Gemini 2.5 web_search should bill $35/1K queries")
	require.Less(t, policy3x.pricing["web_search"], policy25.pricing["web_search"],
		"Gemini 3.x web search must be cheaper than 2.5")
}
