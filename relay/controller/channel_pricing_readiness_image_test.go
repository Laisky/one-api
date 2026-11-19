package controller

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestChannelPricingReadinessMatchesImageAdmission is a differential test of
// the editor warning against the real image relay: for strict GPT Image IDs on
// native OpenAI and on Azure (where 2.6 is also operator-priced), and for every
// tariff shape, the readiness gaps computed from the saved channel row predict
// the relay's outcome exactly. An image-fallback gap means 503
// image_billing_not_configured before reservation, an operator-tariff gap (and
// no image gap) means a dispatch refusal without provider contact, and no gap
// means the request is served.
func TestChannelPricingReadinessMatchesImageAdmission(t *testing.T) {
	useProductionGlobalPricing(t)
	tariffs := []struct {
		name  string
		local *model.ModelConfigLocal
	}{
		{name: "unpriced"},
		{name: "token_price_only", local: &model.ModelConfigLocal{Ratio: 2, CompletionRatio: 10}},
		{name: "image_price_only", local: &model.ModelConfigLocal{Image: &model.ImagePricingLocal{PricePerImageUsd: 0.04}}},
		{name: "request_defaults_only", local: &model.ModelConfigLocal{MaxTokens: 4096, Image: &model.ImagePricingLocal{DefaultSize: "1024x1024"}}},
		{name: "priced", local: &nativeOnlyImageOperatorTariff},
	}
	hosts := []struct {
		name    string
		channel int
		config  model.ChannelConfig
		models  []string
	}{
		{name: "openai", channel: channeltype.OpenAI, models: []string{"gpt-image-2.5-sunburst", "gpt-image-2.6-flare"}},
		{name: "azure", channel: channeltype.Azure, config: model.ChannelConfig{APIVersion: "2025-04-01-preview"},
			models: []string{"gpt-image-2.6-sunburst", "gpt-image-2.6-flare"}},
	}
	outcomes := map[string]int{}
	for _, host := range hosts {
		for _, imageModel := range host.models {
			for _, tariff := range tariffs {
				t.Run(fmt.Sprintf("%s/%s/%s", host.name, imageModel, tariff.name), func(t *testing.T) {
					const balance = int64(1_000_000)
					xaiVideoSetup(t, balance, false)
					upstream := newTariffUpstream(t, func(string) (string, string) { return "application/json", nativeOnlyImageReceipt })
					c, _, requestID := protocolContext(t, host.channel, imageModel, "/v1/images/generations", `{"model":"alias","prompt":"A lighthouse"}`, upstream.server.URL, balance, 1, false, tariff.local)
					c.Set(ctxkey.Config, host.config)
					clearNativeSettledLedger(t, requestID)

					// The editor evaluates exactly the saved row the relay reads.
					channel := c.MustGet(ctxkey.ChannelModel).(*model.Channel)
					configs, err := model.ParseModelPriceConfigs(channel.ModelConfigs)
					require.NoError(t, err)
					mapping := c.GetStringMapString(ctxkey.ModelMapping)
					gaps := EvaluateChannelPricingReadiness(ResolveChannelPricingAdaptor(host.channel), []string{"alias"}, mapping, configs, readinessAt)
					var imageGap, tokenGap bool
					for _, gap := range gaps {
						require.Equal(t, "alias", gap.Model)
						require.Equal(t, imageModel, gap.UpstreamModel)
						imageGap = imageGap || gap.Requirement == PricingRequirementImageFallback
						tokenGap = tokenGap || gap.Requirement == PricingRequirementTokenTariff
					}

					apiErr := RelayImageHelper(c, relaymode.ImagesGenerations)
					drainCriticalTasks(t)
					switch {
					case imageGap:
						outcomes["image_gap"]++
						require.NotNil(t, apiErr)
						require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
						require.Equal(t, "image_billing_not_configured", apiErr.Error.Code)
					case tokenGap:
						outcomes["token_gap"]++
						require.NotNil(t, apiErr)
						require.Contains(t, apiErr.Error.Message, fmt.Sprintf("model %q requires explicit positive input and output pricing in channel model_configs", imageModel))
					default:
						outcomes["ready"]++
						require.Nil(t, apiErr, "a channel the editor reports ready must be served")
						require.EqualValues(t, 1, upstream.calls.Load())
						return
					}
					require.Zero(t, upstream.calls.Load(), "a refused request never reaches the provider")
					require.Equal(t, balance, reloadUserQuota(t), "a refused request keeps the balance")
				})
			}
		}
	}
	require.Positive(t, outcomes["image_gap"], "control")
	require.Positive(t, outcomes["token_gap"], "control")
	require.Positive(t, outcomes["ready"], "control")
}
