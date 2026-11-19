package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// nativeOnlyImageReceipt is an Images API receipt of 10 text input tokens and
// 100 output image tokens, fully classified so no upper-bound estimate applies.
const nativeOnlyImageReceipt = `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":{"input_tokens":10,"output_tokens":100,"total_tokens":110,"input_tokens_details":{"text_tokens":10,"image_tokens":0}}}`

// nativeOnlyImageCachedReceipt is a receipt of 1,000 text input tokens, 400 of
// them cached, and 100 output image tokens.
const nativeOnlyImageCachedReceipt = `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":{"input_tokens":1000,"output_tokens":100,"total_tokens":1100,"input_tokens_details":{"text_tokens":1000,"image_tokens":0,"cached_tokens":400}}}`

// nativeOnlyImageOperatorTariff is a deliberately non-native operator tariff:
// $4/M text input (ratio 2) and $40/M image output (completion ratio 10), with
// $0.04 per image as the reservation/fallback. OpenAI's native GPT Image 2.6
// tariff is $5/M input, $1.25/M cached input and $30/M output.
var nativeOnlyImageOperatorTariff = model.ModelConfigLocal{Ratio: 4 * billingratio.MilliTokensUsd, CompletionRatio: 10, Image: &model.ImagePricingLocal{PricePerImageUsd: 0.04}}

// nativeOnlyImageTariffs are the channel configurations every host is probed
// with; priced cases carry the receipt and its hand-computed charge.
var nativeOnlyImageTariffs = []struct {
	name, receipt string
	local         *model.ModelConfigLocal
	charge        int64
}{
	{name: "unpriced"},
	{name: "token_price_only", local: &model.ModelConfigLocal{Ratio: 4 * billingratio.MilliTokensUsd, CompletionRatio: 10}},
	{name: "image_price_only", local: &model.ModelConfigLocal{Image: &model.ImagePricingLocal{PricePerImageUsd: 0.04}}},
	// ceil(10 text tokens * 2 + 100 output tokens * 2 * 10) = 20 + 2,000.
	{name: "priced", receipt: nativeOnlyImageReceipt, local: &nativeOnlyImageOperatorTariff, charge: 2_020},
	// The channel sets no cache price, so cached input is billed at the channel
	// input price; OpenAI's native cached rate must never leak in:
	// ceil(1,000 * 2 + 100 * 2 * 10) = 4,000 (the native cache rate would give 3,450).
	{name: "priced_cached_receipt", receipt: nativeOnlyImageCachedReceipt, local: &nativeOnlyImageOperatorTariff, charge: 4_000},
}

// TestNativeOnlyImageOperatorTariffHTTP drives GPT Image 2.6 generations for the
// native-only IDs through RelayImageHelper against a fake TLS provider and the
// SQLite ledger. On Azure: without image.price_per_image_usd the request is
// refused before reservation; with only a per-image price it is reserved and
// then refused by the operator-tariff dispatch gate with a full refund; with a
// complete operator tariff it is dispatched once, reserves the per-image
// fallback, and settles the receipt at the channel's token prices, never at
// OpenAI's native token or cached-input prices. The AIProxy
// library host does not serve images (its default endpoints exclude them, so
// the distributor never routes there); if forced through the controller, every
// incompletely priced configuration is still refused without provider contact.
func TestNativeOnlyImageOperatorTariffHTTP(t *testing.T) {
	useProductionGlobalPricing(t)
	require.True(t, channeltype.IsEndpointSupported(relaymode.ImagesGenerations, channeltype.DefaultEndpointsForChannelType(channeltype.Azure)))
	require.False(t, channeltype.IsEndpointSupported(relaymode.ImagesGenerations, channeltype.DefaultEndpointsForChannelType(channeltype.AIProxyLibrary)),
		"the AIProxy library API has no image endpoint")
	// ceil($0.04 * 500,000 quota/USD * size/quality tier 1 * group 1) * n=1.
	const reservation = int64(20_000)
	for _, host := range []struct {
		name    string
		channel int
		config  model.ChannelConfig
	}{
		{name: "azure", channel: channeltype.Azure, config: model.ChannelConfig{APIVersion: "2025-04-01-preview"}},
		{name: "aiproxy", channel: channeltype.AIProxyLibrary, config: model.ChannelConfig{LibraryID: "library-fixture"}},
	} {
		for _, imageModel := range []string{"gpt-image-2.6-sunburst", "gpt-image-2.6-flare"} {
			for _, tariff := range nativeOnlyImageTariffs {
				if host.channel == channeltype.AIProxyLibrary && tariff.charge > 0 {
					continue // Unsupported by design: asserted through the endpoint gate above.
				}
				t.Run(fmt.Sprintf("%s/%s/%s", host.name, imageModel, tariff.name), func(t *testing.T) {
					const balance = int64(1_000_000)
					xaiVideoSetup(t, balance, false)
					receipt := tariff.receipt
					if receipt == "" {
						receipt = nativeOnlyImageReceipt
					}
					upstream := newTariffUpstream(t, func(string) (string, string) { return "application/json", receipt })
					c, writer, requestID := protocolContext(t, host.channel, imageModel, "/v1/images/generations", `{"model":"alias","prompt":"A lighthouse"}`, upstream.server.URL, balance, 1, false, tariff.local)
					c.Set(ctxkey.Config, host.config)
					clearNativeSettledLedger(t, requestID)

					apiErr := RelayImageHelper(c, relaymode.ImagesGenerations)
					drainCriticalTasks(t)
					logQuota, costQuota := tariffRequestLedger(t, requestID)
					switch tariff.name {
					case "unpriced", "token_price_only":
						require.NotNil(t, apiErr)
						require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
						require.Equal(t, "image_billing_not_configured", apiErr.Error.Code)
						require.Contains(t, apiErr.Error.Message, "image.price_per_image_usd")
					case "image_price_only":
						// Reserved at the per-image fallback, then refused at dispatch.
						require.NotNil(t, apiErr)
						require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
						require.Equal(t, "do_request_failed", apiErr.Error.Code)
						require.Contains(t, apiErr.Error.Message, fmt.Sprintf("model %q requires explicit positive input and output pricing in channel model_configs", imageModel))
					default:
						require.Nil(t, apiErr)
						require.EqualValues(t, 1, upstream.calls.Load(), "a priced image request is dispatched exactly once")
						obs := upstream.next(t)
						require.Equal(t, "/openai/deployments/"+imageModel+"/images/generations", obs.Path)
						require.Contains(t, obs.Body, `"model":"`+imageModel+`"`)
						require.Contains(t, writer.Body.String(), "b64_json")
						require.Equal(t, reservation, balance-obs.UserQuota, "admission must reserve the operator per-image fallback")
						require.Equal(t, balance-tariff.charge, reloadUserQuota(t), "the receipt must settle at the channel token tariff")
						require.Equal(t, tariff.charge, consumeLogQuota(t, requestID))
						require.Equal(t, tariff.charge, logQuota)
						require.Equal(t, tariff.charge, costQuota)
						return
					}
					require.Zero(t, upstream.calls.Load(), "the provider must never be contacted without a complete operator tariff")
					require.Equal(t, balance, reloadUserQuota(t), "any reservation must be refunded in full")
					require.Zero(t, consumeLogQuota(t, requestID))
					require.Zero(t, logQuota)
					require.Zero(t, costQuota)
				})
			}
		}
	}
}
