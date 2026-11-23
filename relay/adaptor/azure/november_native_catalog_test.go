package azure

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestNativeNovemberModelsRequireFoundryEvidence prevents new native OpenAI
// prices from being presented as Azure prices. Parameter t asserts; none return.
func TestNativeNovemberModelsRequireFoundryEvidence(t *testing.T) {
	t.Parallel()
	provider := &Adaptor{}
	provider.Init(&meta.Meta{ChannelType: channeltype.Azure})
	fallback := &adaptor.DefaultPricingMethods{}
	for _, name := range []string{"gpt-6.2-astra", "gpt-6.2-sol", "gpt-6-cyber", "gpt-image-2.6-sunburst", "gpt-image-2.6-flare"} {
		require.Contains(t, openai.ModelList, name)
		require.NotContains(t, provider.GetModelList(), name)
		require.NotContains(t, provider.GetDefaultModelPricing(), name)
		require.Equal(t, fallback.GetModelRatio(name), provider.GetModelRatio(name))
		require.Equal(t, fallback.GetCompletionRatio(name), provider.GetCompletionRatio(name))
	}
	for _, name := range []string{"gpt-6.1-terra", "gpt-image-2.5-sunburst", "claude-fable-5-2"} {
		require.Contains(t, provider.GetModelList(), name)
		require.Contains(t, provider.GetDefaultModelPricing(), name)
	}
	// Catalog evidence does not block explicitly mapped administrator deployments.
	url, err := provider.GetRequestURL(&meta.Meta{ChannelType: channeltype.Azure,
		ActualModelName: "private-image-deployment", OriginModelName: "gpt-image-2.6-flare",
		BaseURL: "https://example.openai.azure.com", Mode: relaymode.ImagesGenerations})
	require.NoError(t, err)
	require.Contains(t, url, "/openai/deployments/private-image-deployment/images/generations")
}
