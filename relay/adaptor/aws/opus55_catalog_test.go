package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	claude "github.com/Laisky/one-api/relay/adaptor/aws/claude"
	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestOpus55CatalogAndProfiles checks the complete discovery -> family -> native
// ID -> inference-profile path without account credentials or inference calls.
func TestOpus55CatalogAndProfiles(t *testing.T) {
	t.Parallel()
	const id = "claude-opus-5-5"
	const native = "anthropic.claude-opus-5-5"
	a := &Adaptor{}
	require.Contains(t, a.GetModelList(), id)
	require.NotNil(t, GetAdaptor(id))
	require.True(t, IsClaudeModel(id))
	require.Equal(t, native, claude.AwsModelIDMap[id])
	cfg, ok := a.GetDefaultModelPricing()[id]
	require.True(t, ok)
	require.InDelta(t, 4*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 20*ratio.MilliTokensUsd, cfg.Ratio*cfg.CompletionRatio, 1e-12)
	require.InDelta(t, .2*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
	require.InDelta(t, 5*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 8*ratio.MilliTokensUsd, cfg.CacheWrite1hRatio, 1e-12)
	require.NotContains(t, cfg.SupportedFeatures, "structured_outputs", "Bedrock card differs from first-party Claude")
	for _, region := range []string{"us-east-1", "eu-west-1", "ap-northeast-1", "ca-central-1"} {
		require.Equal(t, "global."+native, utils.ConvertModelID2CrossRegionProfile(context.Background(), native, region))
	}
	for _, prefix := range []string{"global", "us", "eu", "au", "jp"} {
		profile := prefix + "." + native
		require.Equal(t, profile, utils.ConvertModelID2CrossRegionProfile(context.Background(), profile, "us-east-1"))
	}
	require.NotContains(t, utils.GlobalProfileSourceRegions[native], "us-gov-west-1")
}
