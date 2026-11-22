package aiproxy

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
)

// TestLibraryCatalogRequiresHostEvidence ensures new native IDs do not acquire
// unverified library prices. Parameter t reports assertions; nothing returns.
func TestLibraryCatalogRequiresHostEvidence(t *testing.T) {
	t.Parallel()
	provider := &Adaptor{}
	fallback := &adaptor.DefaultPricingMethods{}
	for _, name := range []string{"gpt-6.2-astra", "gpt-6.2-sol", "gpt-6-cyber", "gpt-image-2.6-sunburst", "gpt-image-2.6-flare"} {
		require.Contains(t, openai.ModelList, name)
		require.NotContains(t, provider.GetModelList(), name)
		require.NotContains(t, provider.GetDefaultModelPricing(), name)
		require.Equal(t, fallback.GetModelRatio(name), provider.GetModelRatio(name))
		require.Equal(t, fallback.GetCompletionRatio(name), provider.GetCompletionRatio(name))
	}
	require.Equal(t, openai.ModelRatios["gpt-4o-mini"], provider.GetDefaultModelPricing()["gpt-4o-mini"])
	url, err := provider.GetRequestURL(&meta.Meta{BaseURL: "https://library.example", ActualModelName: "operator-model"})
	require.NoError(t, err)
	require.Equal(t, "https://library.example/api/library/ask", url)
}
