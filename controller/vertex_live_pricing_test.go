package controller

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// vertexLivePrices supplies deliberately distinctive administrator prices.
// Parameters: none. Returns: text input/output 1/3 quota units per token,
// audio input/output 4/20, and image/video input 6; these are not provider quotes.
func vertexLivePrices() dbmodel.ModelConfigLocal {
	return dbmodel.ModelConfigLocal{
		Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 3,
		Audio: &dbmodel.AudioPricingLocal{PromptRatio: 4, CompletionRatio: 5},
		Image: &dbmodel.ImagePricingLocal{PromptRatio: 6},
	}
}

// TestVertexLiveExplicitPricingAndSettlement exercises the production pricing
// resolver, reservation and receipt settlement with independent hand-calculated
// values. Parameters: t owns the test. Returns: none; no provider calls occur.
func TestVertexLiveExplicitPricingAndSettlement(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gemini-3.8-live", "operator-configured-live-id"} {
		t.Run(name, func(t *testing.T) {
			m := vertexLiveFixtureMeta(name)
			m.StartTime = time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
			provider := resolveRealtimePricingAdaptor(m)
			require.IsType(t, &vertexai.Adaptor{}, provider)
			_, err := estimateRealtimeSessionReservation(m, 1, 1, nil, provider)
			require.ErrorContains(t, err, "channel pricing")
			cfg := map[string]dbmodel.ModelConfigLocal{name: vertexLivePrices()}
			reserved, err := estimateRealtimeSessionReservation(m, 1, 1, cfg, provider)
			require.NoError(t, err)
			require.EqualValues(t, 72000, reserved, "3000 audio tokens times (4 + 20)")
			free, err := estimateRealtimeSessionReservation(m, 1, 0, cfg, provider)
			require.NoError(t, err)
			require.Zero(t, free)
			collector := realtime.NewGeminiLedger()
			require.NoError(t, collector.Observe([]byte(`{"serverContent":{"modelTurn":{},"turnComplete":true},"usageMetadata":{"promptTokenCount":400,"responseTokenCount":200,"totalTokenCount":600,"thoughtsTokenCount":20,"promptTokensDetails":[{"modality":"TEXT","tokenCount":100},{"modality":"AUDIO","tokenCount":100},{"modality":"IMAGE","tokenCount":100},{"modality":"VIDEO","tokenCount":100}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":100},{"modality":"AUDIO","tokenCount":100}]}}`)))
			input := quota.ComputeInput{Usage: &relaymodel.Usage{Realtime: collector.Finish(false)}, ModelName: name,
				ModelRatio: 1, GroupRatio: 1, ChannelModelConfigs: cfg, PricingAdaptor: provider, RequestTime: m.StartTime}
			result, metadata := prepareRealtimeReceiptSettlement(input, reserved, logger.Logger)
			require.False(t, result.UnpricedUsage)
			require.EqualValues(t, 4000, result.TotalQuota, "100 * (1 + 4 + 6 + 6 + 3 + 20)")
			require.Equal(t, true, metadata["realtime_billing_complete"])
			require.Equal(t, 400, result.PromptTokens)
			require.Equal(t, 200, result.CompletionTokens)
			input.GroupRatio = 2
			require.EqualValues(t, 8000, quota.Compute(input).TotalQuota)
			input.GroupRatio = 0
			require.Zero(t, quota.Compute(input).TotalQuota)
			input.GroupRatio = 1
			input.Usage = &relaymodel.Usage{Realtime: realtime.NewGeminiLedger().Finish(false)}
			idle, _ := prepareRealtimeReceiptSettlement(input, reserved, logger.Logger)
			require.Zero(t, idle.TotalQuota)
			input.Usage = &relaymodel.Usage{Realtime: realtime.NewGeminiLedger().Finish(true)}
			unresolved, metadata := prepareRealtimeReceiptSettlement(input, reserved, logger.Logger)
			require.Equal(t, reserved, unresolved.TotalQuota)
			require.Equal(t, false, metadata["realtime_billing_complete"])
		})
	}
}

// TestVertexLiveIncompletePricingCannotStartWork verifies that published catalog
// entries cannot silently borrow another backend's rates. Parameters: t owns
// the test. Returns: none; zero input pricing remains explicitly configurable.
func TestVertexLiveIncompletePricingCannotStartWork(t *testing.T) {
	t.Parallel()
	m := vertexLiveFixtureMeta("gemini-3.8-live")
	provider := resolveRealtimePricingAdaptor(m)
	for _, tc := range []struct {
		name string
		edit func(*dbmodel.ModelConfigLocal)
	}{
		{"missing audio", func(c *dbmodel.ModelConfigLocal) { c.Audio = nil }},
		{"zero audio output", func(c *dbmodel.ModelConfigLocal) { c.Audio.CompletionRatio = 0 }},
		{"missing visual rate", func(c *dbmodel.ModelConfigLocal) { c.Image = nil }},
		{"negative visual rate", func(c *dbmodel.ModelConfigLocal) { c.Image.PromptRatio = -1 }},
		{"NaN output rate", func(c *dbmodel.ModelConfigLocal) { c.CompletionRatio = math.NaN() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := vertexLivePrices()
			tc.edit(&cfg)
			_, err := estimateRealtimeSessionReservation(m, 1, 1, map[string]dbmodel.ModelConfigLocal{m.ActualModelName: cfg}, provider)
			require.Error(t, err)
		})
	}
	free, err := estimateRealtimeSessionReservation(m, 0, 1,
		map[string]dbmodel.ModelConfigLocal{m.ActualModelName: {}}, provider)
	require.NoError(t, err)
	require.Zero(t, free)
}
