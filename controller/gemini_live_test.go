package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiRealtimeChannelAndReservation checks protocol selection and actual
// reservation pricing. Parameters: t is the test handle. Returns: none.
func TestGeminiRealtimeChannelAndReservation(t *testing.T) {
	t.Parallel()
	for _, channel := range []int{channeltype.Gemini, channeltype.GeminiOpenAICompatible} {
		m := &meta.Meta{ChannelType: channel, APIType: apitype.OpenAI, Mode: relaymode.Realtime, ActualModelName: "gemini-3.8-live", APIKey: "fixture-not-a-secret", StartTime: time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)}
		require.True(t, isGeminiLiveRequest(m))
		require.NoError(t, validateGeminiRealtimeTransport(m))
		provider := resolveRealtimePricingAdaptor(m)
		require.IsType(t, &gemini.Adaptor{}, provider)
		reserved, err := estimateRealtimeSessionReservation(m, .75*ratio.MilliTokensUsd, 1, nil, provider)
		require.NoError(t, err)
		require.EqualValues(t, 22500, reserved)
		free, err := estimateRealtimeSessionReservation(m, .75*ratio.MilliTokensUsd, 0, nil, provider)
		require.NoError(t, err)
		require.Zero(t, free)
		// Catalog membership is no longer a transport admission rule. The
		// upstream decides model support; unknown prices still require config.
		m.ActualModelName = "operator-configured-live-id"
		require.NoError(t, validateGeminiRealtimeTransport(m))
		_, err = estimateRealtimeSessionReservation(m, .75*ratio.MilliTokensUsd, 1, nil, provider)
		require.Error(t, err)
		m.ActualModelName = "../other-project/model"
		require.Error(t, validateGeminiRealtimeTransport(m))
	}
	// A Vertex channel still needs actual local project/credential configuration.
	require.Error(t, validateGeminiRealtimeTransport(&meta.Meta{ChannelType: channeltype.VertextAI, ActualModelName: "gemini-3.8-live"}))
	require.NoError(t, validateGeminiRealtimeTransport(&meta.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "gpt-realtime"}))
}

// TestGeminiReceiptSettlementBehavior uses the same settlement preparation as
// the database path. Parameters: t is a test. Returns: none. Measured sessions
// refund excess reservations; missing receipts retain a labeled estimate only.
func TestGeminiReceiptSettlementBehavior(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name             string
		active, measured bool
		group            float64
		want             int64
		estimated        bool
	}{
		{"idle_refund", false, false, 1, 0, false},
		{"measured_audio_and_transcript", false, true, 1, 975, false},
		{"missing_receipt_retains_reservation", true, false, 1, 22500, true},
		{"partial_plus_unresolved", true, true, 1, 22500, true},
		{"free_group_remains_free", true, true, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := realtime.NewGeminiLedger()
			if tc.measured {
				require.NoError(t, g.Observe([]byte(`{"serverContent":{"modelTurn":{},"turnComplete":true},"usageMetadata":{"promptTokenCount":100,"responseTokenCount":200,"totalTokenCount":300,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":100}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":100},{"modality":"AUDIO","tokenCount":100}]}}`)))
			}
			usage := &relaymodel.Usage{Realtime: g.Finish(tc.active)}
			result, metadata := prepareRealtimeReceiptSettlement(quota.ComputeInput{Usage: usage, ModelName: "gemini-3.8-live", ModelRatio: .75 * ratio.MilliTokensUsd, GroupRatio: tc.group, PricingAdaptor: &gemini.Adaptor{}}, 22500, logger.Logger)
			require.Equal(t, tc.want, result.TotalQuota)
			if tc.estimated {
				require.Equal(t, false, metadata["realtime_billing_complete"])
				require.Equal(t, result.TotalQuota, int64(22500))
			} else if !tc.active {
				require.Equal(t, true, metadata["realtime_billing_complete"])
			}
		})
	}
}
