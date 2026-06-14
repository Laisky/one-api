package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestIsDeepSeekUpstream verifies the channel-upstream detection used to scope
// DeepSeek-specific handling in the Response API fallback.
func TestIsDeepSeekUpstream(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		meta *metalib.Meta
		want bool
	}{
		{"nil", nil, false},
		{"dedicated deepseek channel", &metalib.Meta{ChannelType: channeltype.DeepSeek}, true},
		{"custom channel pointed at deepseek", &metalib.Meta{ChannelType: channeltype.OpenAICompatible, BaseURL: "https://api.deepseek.com"}, true},
		{"deepseek url mixed case", &metalib.Meta{ChannelType: channeltype.OpenAICompatible, BaseURL: "https://API.DeepSeek.com/v1"}, true},
		{"deepseek path on neutral host", &metalib.Meta{ChannelType: channeltype.OpenAICompatible, BaseURL: "https://proxy.example.com/deepseek/v1"}, false},
		{"deepseek suffix trap", &metalib.Meta{ChannelType: channeltype.OpenAICompatible, BaseURL: "https://api.notdeepseek.com"}, false},
		{"nvidia hosting deepseek weights", &metalib.Meta{ChannelType: channeltype.NVIDIA, BaseURL: "https://integrate.api.nvidia.com/v1"}, false},
		{"novita", &metalib.Meta{ChannelType: channeltype.Novita, BaseURL: "https://api.novita.ai/v3/openai"}, false},
		{"empty base url non-deepseek channel", &metalib.Meta{ChannelType: channeltype.OpenAICompatible, BaseURL: ""}, false},
	}
	for _, tc := range cases {
		require.Equalf(t, tc.want, isDeepSeekUpstream(tc.meta), "case %s", tc.name)
	}
}

// TestShouldRouteResponseFallbackThroughDeepSeek verifies the flip only engages
// when BOTH the model is a DeepSeek model AND the upstream is DeepSeek's API.
func TestShouldRouteResponseFallbackThroughDeepSeek(t *testing.T) {
	t.Parallel()

	// Real DeepSeek channel -> route through DeepSeek adaptor (no-op: apitype is
	// already DeepSeek, but the predicate must still be true).
	require.True(t, shouldRouteResponseFallbackThroughDeepSeek(&metalib.Meta{
		ChannelType:     channeltype.DeepSeek,
		BaseURL:         "https://api.deepseek.com",
		ActualModelName: "deepseek-chat",
	}))

	// Custom channel pointed at DeepSeek -> preserve DeepSeek request conversion.
	require.True(t, shouldRouteResponseFallbackThroughDeepSeek(&metalib.Meta{
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://api.deepseek.com",
		OriginModelName: "DeepSeek-Coder",
	}))

	// NVIDIA hosting DeepSeek open weights -> MUST NOT flip (the bug). Model name
	// matches but the upstream is NVIDIA.
	require.False(t, shouldRouteResponseFallbackThroughDeepSeek(&metalib.Meta{
		ChannelType:     channeltype.NVIDIA,
		BaseURL:         "https://integrate.api.nvidia.com/v1",
		ActualModelName: "deepseek-ai/deepseek-v4-flash",
	}))

	// DeepSeek upstream but non-DeepSeek model -> no flip.
	require.False(t, shouldRouteResponseFallbackThroughDeepSeek(&metalib.Meta{
		ChannelType:     channeltype.DeepSeek,
		BaseURL:         "https://api.deepseek.com",
		ActualModelName: "some-other-model",
	}))
}

// TestResponseFallbackPreservesNvidiaFreePricingForDeepSeekModel is the
// regression test for the billing bug: a Response API request to a NVIDIA
// channel for a (free) deepseek-ai/* model must keep NVIDIA's adaptor and free
// pricing, not get hijacked onto the DeepSeek adaptor's paid pricing.
func TestResponseFallbackPreservesNvidiaFreePricingForDeepSeekModel(t *testing.T) {
	t.Parallel()

	const model = "deepseek-ai/deepseek-v4-flash"

	// Pre-condition (documents the latent trap): the model name DOES match the
	// DeepSeek heuristic, so the unscoped flip would have fired on it.
	require.True(t, isDeepSeekModel(model), "model name matches the DeepSeek prefix heuristic")

	meta := &metalib.Meta{
		ChannelType:     channeltype.NVIDIA,
		BaseURL:         "https://integrate.api.nvidia.com/v1",
		ActualModelName: model,
		OriginModelName: model,
		APIType:         channeltype.ToAPIType(channeltype.NVIDIA),
	}

	// The guard prevents the flip, so the API type stays NVIDIA.
	if shouldRouteResponseFallbackThroughDeepSeek(meta) {
		meta.APIType = apitype.DeepSeek
	}
	require.Equal(t, apitype.NVIDIA, meta.APIType, "NVIDIA channel must not be flipped to the DeepSeek adaptor")

	// Pricing therefore resolves through the NVIDIA adaptor, which bills the
	// model free (Ratio 0) — not the DeepSeek adaptor / 2.5 USD default.
	pricingAdaptor := resolvePricingAdaptor(meta)
	require.NotNil(t, pricingAdaptor)
	require.Equal(t, "nvidia", pricingAdaptor.GetChannelName())
	require.Equal(t, 0.0, pricingAdaptor.GetModelRatio(model), "NVIDIA-hosted deepseek model must remain free")

	// Sanity: a real DeepSeek channel still resolves to the DeepSeek adaptor.
	dsMeta := &metalib.Meta{
		ChannelType: channeltype.DeepSeek,
		BaseURL:     "https://api.deepseek.com",
		APIType:     channeltype.ToAPIType(channeltype.DeepSeek),
	}
	require.Equal(t, "deepseek", resolvePricingAdaptor(dsMeta).GetChannelName())
}
