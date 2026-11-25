package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// TestVerbatimImageBodiesHoldTheClientCount drives per-image billed models
// whose body is forwarded verbatim through RelayImageHelper and the ledger. The
// provider receives the client's own n, which the channel's max_images clamp
// only applied to the gateway's copy, so the hold and the charge without a
// usable receipt must cover the client's n: under an opt-in megapixel tariff
// and under a flat per-image price alike. A client n below min_images keeps
// the normalized minimum, since a provider may render its own minimum.
// Parameters: t runs the cases. Returns: none.
func TestVerbatimImageBodiesHoldTheClientCount(t *testing.T) {
	const balance = int64(10_000_000)
	for _, tc := range []struct {
		name, model, body, configs string
		channelType                int
		perImage, count            int64
	}{
		{
			// FLUX.2 Pro on DeepInfra: $0.03 per 1 MP image, max_images 4.
			name: "megapixel_tariff_above_max_images", channelType: channeltype.DeepInfra, model: "black-forest-labs/FLUX-2-pro",
			body:     `{"model":"black-forest-labs/FLUX-2-pro","prompt":"A lighthouse","n":6,"size":"1024x1024"}`,
			perImage: 15_000, count: 6,
		},
		{
			// gpt-image-1.5 high 1024x1024 is $0.133 per image; the channel
			// clamps the gateway's copy to 2 images.
			name: "flat_price_above_max_images", channelType: channeltype.OpenAICompatible, model: "gpt-image-1.5",
			body:     `{"model":"gpt-image-1.5","prompt":"A lighthouse","n":3,"size":"1024x1024","quality":"high"}`,
			configs:  `{"gpt-image-1.5":{"image":{"max_images":2}}}`,
			perImage: 66_500, count: 3,
		},
		{
			// A client n below min_images keeps the normalized minimum.
			name: "flat_price_below_min_images", channelType: channeltype.OpenAICompatible, model: "gpt-image-1.5",
			body:     `{"model":"gpt-image-1.5","prompt":"A lighthouse","n":1,"size":"1024x1024","quality":"high"}`,
			configs:  `{"gpt-image-1.5":{"image":{"min_images":2,"max_images":4}}}`,
			perImage: 66_500, count: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runStrictImageRelay(t, strictImageRelayCase{
				requestID: "verbatim-count-" + tc.name, channelType: tc.channelType, model: tc.model,
				path: "/v1/images/generations", contentType: "application/json", body: []byte(tc.body),
				configs: tc.configs, usage: `null`, balance: balance,
			})
			require.Nil(t, res.err)
			require.Equal(t, int32(1), res.calls)
			require.JSONEq(t, tc.body, string(res.sentBody), "the body reaches the provider verbatim")
			hold := tc.perImage * tc.count
			require.Equal(t, balance-hold, res.userAtUpstream, "the hold covers the images the provider is asked for")
			require.Equal(t, balance-hold, res.userQuota, "without a receipt the hold is charged")
			require.Equal(t, balance-hold, res.tokenQuota)
			require.NotNil(t, res.consume)
			require.Equal(t, int(hold), res.consume.Quota)
		})
	}
}
