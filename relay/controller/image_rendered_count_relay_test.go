package controller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// TestStrictGPTImageHoldsTheRenderedCount checks that a strict GPT Image hold
// and its missing-receipt charge count the images the provider is actually
// asked to render. A re-encoded body carries the normalized, clamped n. A body
// forwarded verbatim carries the client's n, so a channel min_images raised
// only in the gateway's normalized copy must not be held or charged, and an n
// above max_images that reaches the provider unclamped must be held in full,
// with either the derived default or an operator per-image price. Parameters:
// t runs the cases. Returns: none.
func TestStrictGPTImageHoldsTheRenderedCount(t *testing.T) {
	const balance = int64(10_000_000)
	const prompt = "A lighthouse"
	// 0.1 USD at 500,000 quota/USD is a 50,000 quota hold per image.
	const perImage = int64(50_000)
	for _, tc := range []struct {
		name, configs, sentN string
		channelType, n       int
		hold                 int64
		source               string
	}{
		{
			// The client asks for one image; the gateway's min_images raises
			// only its own copy, the provider renders one.
			name: "verbatim_default_hold_ignores_gateway_min_images", channelType: channeltype.OpenAICompatible, n: 1,
			configs: `{"gpt-image-2.5-flare":{"image":{"min_images":2}}}`,
			hold:    expectedDefaultImageHold(1756, 30, 5, len(prompt), 1), sentN: `"n":1`, source: "default_fallback",
		},
		{
			name: "verbatim_priced_hold_ignores_gateway_min_images", channelType: channeltype.OpenAICompatible, n: 1,
			configs: `{"gpt-image-2.5-flare":{"image":{"price_per_image_usd":0.1,"min_images":2}}}`,
			hold:    perImage, sentN: `"n":1`, source: "configured_fallback",
		},
		{
			// The catalog clamps n to 10, but the verbatim body asks for 12.
			name: "verbatim_priced_hold_covers_unclamped_n", channelType: channeltype.OpenAICompatible, n: 12,
			configs: `{"gpt-image-2.5-flare":{"image":{"price_per_image_usd":0.1}}}`,
			hold:    12 * perImage, sentN: `"n":12`, source: "configured_fallback",
		},
		{
			// Control: a re-encoded native body sends the clamped n.
			name: "reencoded_priced_hold_uses_clamped_n", channelType: channeltype.OpenAI, n: 12,
			configs: `{"gpt-image-2.5-flare":{"image":{"price_per_image_usd":0.1}}}`,
			hold:    10 * perImage, sentN: `"n":10`, source: "configured_fallback",
		},
		{
			// Control: a re-encoded native body sends the raised minimum.
			name: "reencoded_priced_hold_uses_raised_minimum", channelType: channeltype.OpenAI, n: 1,
			configs: `{"gpt-image-2.5-flare":{"image":{"price_per_image_usd":0.1,"min_images":2}}}`,
			hold:    2 * perImage, sentN: `"n":2`, source: "configured_fallback",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runStrictImageRelay(t, strictImageRelayCase{
				requestID: "strict-image-rendered-count-" + tc.name, channelType: tc.channelType, model: "gpt-image-2.5-flare",
				path: "/v1/images/generations", contentType: "application/json",
				body:    fmt.Appendf(nil, `{"model":"gpt-image-2.5-flare","prompt":%q,"n":%d,"size":"1024x1024","quality":"high"}`, prompt, tc.n),
				configs: tc.configs, usage: `null`, balance: balance,
			})
			// Without a usable receipt the hold is the charge.
			requireStrictImageSettled(t, res, balance, tc.hold, tc.hold, tc.source)
			require.Contains(t, string(res.sentBody), tc.sentN, "body=%s", res.sentBody)
		})
	}
}

// TestStrictGPTImageChannelEntryCachedTextPrice pins the documented cache-price
// rule of strict GPT Image settlement on a native OpenAI channel: an explicit
// channel entry without cached_input_ratio keeps OpenAI's catalog cached-text
// price ($1.25/M) rather than billing cached text at the entry's own input
// price. (On an operator-priced host there is no catalog price to keep, so
// cached text is billed at the entry's input price; TestNativeOnlyImageOperatorTariffHTTP
// pins that.) Parameters: t runs the case. Returns: none.
func TestStrictGPTImageChannelEntryCachedTextPrice(t *testing.T) {
	const balance = int64(10_000_000)
	res := runStrictImageRelay(t, strictImageRelayCase{
		requestID: "strict-image-entry-cached-text", model: "gpt-image-2.5-flare",
		path: "/v1/images/generations", contentType: "application/json",
		body: []byte(`{"model":"gpt-image-2.5-flare","prompt":"A lighthouse","size":"1024x1024","quality":"high"}`),
		// ratio 5 = $10/M text input, completion ratio 4 = $40/M output.
		configs: `{"gpt-image-2.5-flare":{"ratio":5,"completion_ratio":4,"image":{"price_per_image_usd":0.1}}}`,
		usage:   `{"input_tokens":1000,"output_tokens":100,"total_tokens":1100,"input_tokens_details":{"text_tokens":1000,"image_tokens":0,"cached_tokens":400}}`,
		balance: balance,
	})
	// 600 x $10 + 400 x $1.25 + 100 x $40 = $0.0105; the entry's input price
	// for the cached tokens would give $0.014 (7,000 quota).
	requireStrictImageSettled(t, res, balance, 50_000, 5_250, "provider_tokens")
}
