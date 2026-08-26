package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestZaiAndZhipuBillIndependently is the end-to-end guard for the Zhipu/Z.ai
// split. Both brands advertise glm-4.7, but open.bigmodel.cn prices it in CNY with
// input- and output-length tiers while api.z.ai prices it flat in USD. Because
// resolvePricingAdaptor keys off meta.APIType, each channel must reach its own
// price table for the very same model name.
func TestZaiAndZhipuBillIndependently(t *testing.T) {
	t.Parallel()

	const model = "glm-4.7"
	now := time.Now()

	zaiAdaptor := resolvePricingAdaptor(&metalib.Meta{APIType: apitype.Zai})
	require.NotNil(t, zaiAdaptor)
	require.Equal(t, "zai", zaiAdaptor.GetChannelName())

	zhipuAdaptor := resolvePricingAdaptor(&metalib.Meta{APIType: apitype.Zhipu})
	require.NotNil(t, zhipuAdaptor)
	require.Equal(t, "zhipu", zhipuAdaptor.GetChannelName())

	zaiCfg, ok := pricing.ResolveModelConfig(model, nil, zaiAdaptor, now)
	require.True(t, ok)
	zhipuCfg, ok := pricing.ResolveModelConfig(model, nil, zhipuAdaptor, now)
	require.True(t, ok)

	// Z.AI: flat USD list price, no tiers.
	require.InDelta(t, 0.60*ratio.MilliTokensUsd, zaiCfg.Ratio, 1e-12)
	require.Empty(t, zaiCfg.Tiers)

	// BigModel: CNY, tiered by input and output length.
	require.InDelta(t, 2*ratio.MilliTokensRmb, zhipuCfg.Ratio, 1e-12)
	require.NotEmpty(t, zhipuCfg.Tiers)

	require.NotEqual(t, zaiCfg.Ratio, zhipuCfg.Ratio,
		"the same model id must not bill identically on both brands")

	// The same separation must hold through the ratio-only entry point used by the
	// relay's quota computation.
	require.InDelta(t, 0.60*ratio.MilliTokensUsd,
		pricing.ResolveModelRatioAt(model, nil, nil, zaiAdaptor, now), 1e-12)
	require.InDelta(t, 2*ratio.MilliTokensRmb,
		pricing.ResolveModelRatioAt(model, nil, nil, zhipuAdaptor, now), 1e-12)
}

// TestZaiOnlyModelUnknownToZhipu pins that a Z.AI-exclusive id does not resolve
// against BigModel's table.
func TestZaiOnlyModelUnknownToZhipu(t *testing.T) {
	t.Parallel()

	const zaiOnly = "glm-4-32b-0414-128k"
	now := time.Now()

	zaiAdaptor := resolvePricingAdaptor(&metalib.Meta{APIType: apitype.Zai})
	zhipuAdaptor := resolvePricingAdaptor(&metalib.Meta{APIType: apitype.Zhipu})

	_, ok := pricing.ResolveModelConfig(zaiOnly, nil, zaiAdaptor, now)
	require.True(t, ok)

	_, ok = pricing.ResolveModelConfig(zaiOnly, nil, zhipuAdaptor, now)
	require.False(t, ok, "glm-4-32b-0414-128k is not served by open.bigmodel.cn")
}
