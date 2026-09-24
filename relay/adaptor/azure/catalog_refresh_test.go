package azure_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/azure"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// TestCurrentFoundryClaudeCatalog checks selectable IDs, existing pricing
// dispatch, and native Messages endpoints with t; it returns nothing.
func TestCurrentFoundryClaudeCatalog(t *testing.T) {
	t.Parallel()
	for _, id := range []string{
		"claude-opus-5-5", "claude-opus-5", "claude-fable-5-1", "claude-mythos-5-1",
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			a, native := &azure.Adaptor{}, &anthropic.Adaptor{}
			require.Contains(t, a.GetModelList(), id)
			require.Contains(t, azure.FoundryClaudeModels, id)
			cfg, exists := a.GetDefaultModelPricing()[id]
			require.True(t, exists)
			require.Greater(t, cfg.Ratio, 0.0)
			require.Equal(t, native.GetModelRatio(id), a.GetModelRatio(id))
			require.Equal(t, native.GetCompletionRatio(id), a.GetCompletionRatio(id))
			for _, mapped := range []string{id, "my-custom-deployment"} {
				url, err := a.GetRequestURL(&meta.Meta{
					ChannelType: channeltype.Azure, BaseURL: "https://example.services.ai.azure.com/",
					OriginModelName: id, ActualModelName: mapped,
				})
				require.NoError(t, err)
				require.Equal(t, "https://example.services.ai.azure.com/anthropic/v1/messages", url)
			}
		})
	}
}
