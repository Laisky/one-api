package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestVideoChannelEntryWithoutVideoBlockKeepsProviderTariff drives
// RelayVideoHelper for a per-second video model whose channel model_configs
// entry carries no video block (only a scalar ratio, or a context length).
// Like image and output-video pricing, the request must keep the provider's
// per-second tariff instead of being refused as "video pricing missing", while
// a channel video block still replaces it. Parameters: t runs the cases.
// Returns: none.
func TestVideoChannelEntryWithoutVideoBlockKeepsProviderTariff(t *testing.T) {
	for _, tc := range []struct {
		name   string
		local  model.ModelConfigLocal
		charge int64
	}{
		// sora-2 is $0.10/s: 4 s is $0.40 = 200,000 quota.
		{name: "scalar_ratio_only", local: model.ModelConfigLocal{Ratio: 2}, charge: 200_000},
		{name: "completion_ratio_only", local: model.ModelConfigLocal{CompletionRatio: 3}, charge: 200_000},
		// A channel video block still wins: $0.25/s for 4 s is $1.00.
		{name: "channel_video_block", local: model.ModelConfigLocal{Video: &model.VideoPricingLocal{PerSecondUsd: 0.25}}, charge: 500_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10_000_000)
			xaiVideoSetup(t, balance, false)
			provider := newVideoSecondsProvider(t, "video_channel_fallback_"+tc.name)
			local := tc.local
			c, w, requestID := protocolContext(t, channeltype.OpenAI, "sora-2", "/v1/videos",
				`{"model":"alias","prompt":"A paper boat.","seconds":"4"}`, provider.server.URL, balance, 1, false, &local)

			apiErr := RelayVideoHelper(c)
			drainCriticalTasks(t)

			require.Nil(t, apiErr)
			require.Equal(t, http.StatusOK, w.Code)
			requireOpenAIVideoLedger(t, balance, tc.charge, requestID)
			provider.onlyWire(t)
		})
	}
}
