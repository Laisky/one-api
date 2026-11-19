package quota_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// vertexLive38TariffJSON is the channel model_configs JSON an operator writes
// for the Vertex AI pricing page row "Gemini 3.8 Live API", Non-global
// (https://cloud.google.com/vertex-ai/generative-ai/pricing, fetched
// 2026-11-18): text $0.75 in / $4.50 out, audio $3.00 in / $12.00 out,
// video/image $1.00 in, and "Output: video (avatar)" $1.00 per 1M tokens.
// Every modality price is a ratio to the text input price, so the avatar row is
// video.output_token_ratio = 1.00 / 0.75. The %s verb receives the optional
// video block.
const vertexLive38TariffJSON = `{"gemini-3.8-live": {"ratio": 0.375, "completion_ratio": 6,
	"audio": {"prompt_ratio": 4, "completion_ratio": 4},
	"image": {"prompt_ratio": 1.3333333333333333}%s}}`

// persistedVertexLiveTariff round-trips the operator JSON through the channel
// persistence layer exactly as the admin API stores it. It accepts the test
// handle and the optional video block and returns the configs settlement reads.
// A non-empty block must survive normalization into the stored JSON.
func persistedVertexLiveTariff(t *testing.T, videoBlock string) map[string]modelcfg.ModelConfigLocal {
	t.Helper()
	var authored map[string]modelcfg.ModelConfigLocal
	require.NoError(t, json.Unmarshal([]byte(strings.Replace(vertexLive38TariffJSON, "%s", videoBlock, 1)), &authored))
	channel := &modelcfg.Channel{}
	require.NoError(t, channel.SetModelPriceConfigs(authored))
	require.NotNil(t, channel.ModelConfigs)
	require.Equal(t, strings.Contains(videoBlock, "output_token_ratio"),
		strings.Contains(*channel.ModelConfigs, `"output_token_ratio"`), "stored JSON: %s", *channel.ModelConfigs)
	return channel.GetModelPriceConfigs()
}

// settleLiveReceiptWithTariff settles one raw Gemini Live usageMetadata receipt
// under the Vertex adaptor and the supplied channel configs. It accepts the test
// handle, the configs, and the receipt and returns the decoded record and result.
func settleLiveReceiptWithTariff(t *testing.T, configs map[string]modelcfg.ModelConfigLocal, raw string) (realtime.Record, quota.ComputeResult) {
	t.Helper()
	record, err := realtime.DecodeGeminiUsage(json.RawMessage(raw))
	require.NoError(t, err)
	ledger := realtime.NewLedger()
	ledger.Records = []realtime.Record{record}
	ledger.InputTokens, ledger.OutputTokens = record.Tokens.Input, record.Tokens.Output
	return record, quota.Compute(quota.ComputeInput{
		Usage:               &model.Usage{Realtime: ledger},
		ModelName:           "gemini-3.8-live",
		ModelRatio:          0.75 * ratio.MilliTokensUsd,
		GroupRatio:          1,
		ChannelModelConfigs: configs,
		PricingAdaptor:      &vertexai.Adaptor{},
		RequestTime:         time.Date(2026, 11, 18, 4, 0, 0, 0, time.UTC),
	})
}

// TestVertexLiveVideoOutputTariffSettlesExactly verifies the operator video
// output price end to end through relay/quota: a receipt with VIDEO (avatar)
// output settles exactly at the configured rate and is not flagged, while the
// same receipt under a tariff without the field keeps today's behaviour. It
// accepts t and returns no values.
func TestVertexLiveVideoOutputTariffSettlesExactly(t *testing.T) {
	t.Parallel()
	require.EqualValues(t, 500000, ratio.QuotaPerUsd, "quota scale assumption")
	priced := persistedVertexLiveTariff(t, `, "video": {"output_token_ratio": 1.3333333333333333}`)
	unpriced := persistedVertexLiveTariff(t, "")

	for _, tc := range []struct {
		name, raw string
		// exact is the published charge; legacy is today's charge with every
		// VIDEO token at the cheapest unattributed-output (text) rate.
		exact, legacy int64
	}{
		// 100 x $0.75 + 1000 x $1.00 = $0.001075 = 537.5 quota, rounded up once.
		// Legacy: 100 x $0.75 + 1000 x $4.50 = $0.004575 = 2287.5 quota.
		{"avatar only", `{"promptTokenCount":100,"responseTokenCount":1000,"totalTokenCount":1100,` +
			`"promptTokensDetails":[{"modality":"TEXT","tokenCount":100}],` +
			`"responseTokensDetails":[{"modality":"VIDEO","tokenCount":1000}]}`, 538, 2288},
		// Adds 400 text-out tokens at $4.50: +$0.0018 = +900 quota.
		{"text and avatar", `{"promptTokenCount":100,"responseTokenCount":1400,"totalTokenCount":1500,` +
			`"promptTokensDetails":[{"modality":"TEXT","tokenCount":100}],` +
			`"responseTokensDetails":[{"modality":"TEXT","tokenCount":400},{"modality":"VIDEO","tokenCount":1000}]}`, 1438, 3188},
		// Avatar speech: 600 audio-out tokens at $12.00 = 3600 quota, plus 1000
		// avatar tokens and the 100 text-in tokens.
		{"speech and avatar", `{"promptTokenCount":100,"responseTokenCount":1600,"totalTokenCount":1700,` +
			`"promptTokensDetails":[{"modality":"TEXT","tokenCount":100}],` +
			`"responseTokensDetails":[{"modality":"AUDIO","tokenCount":600},{"modality":"VIDEO","tokenCount":1000}]}`, 4138, 5888},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			record, got := settleLiveReceiptWithTariff(t, priced, tc.raw)
			require.EqualValues(t, 1000, record.Tokens.OutputVideo)
			require.False(t, got.UnpricedUsage, "a configured video output price settles exactly: %v", got.BillingIssues)
			require.Empty(t, got.BillingIssues)
			require.Equal(t, tc.exact, got.TotalQuota)

			_, legacy := settleLiveReceiptWithTariff(t, unpriced, tc.raw)
			require.True(t, legacy.UnpricedUsage, "without the field the receipt stays flagged")
			require.Contains(t, strings.Join(legacy.BillingIssues, "; "), "video output")
			require.Equal(t, tc.legacy, legacy.TotalQuota, "without the field the charge is unchanged")
		})
	}
}

// TestVertexLiveVideoOutputTariffFollowsTimeWindows verifies the new field is
// a ratio to the effective text input price, so an off-peak window that only
// discounts the text ratio discounts video output with it, and a window overlay
// may set its own video output ratio. It accepts t and returns no values.
func TestVertexLiveVideoOutputTariffFollowsTimeWindows(t *testing.T) {
	t.Parallel()
	const window = `, "video": {"output_token_ratio": 1.3333333333333333},
		"time_windows": [{"timezone": "UTC", "ranges": [{"start": "00:00", "end": "12:00"}],
			"overlay": {"ratio": 0.1875%s}}]`
	const avatar = `{"promptTokenCount":100,"responseTokenCount":1000,"totalTokenCount":1100,` +
		`"promptTokensDetails":[{"modality":"TEXT","tokenCount":100}],` +
		`"responseTokensDetails":[{"modality":"VIDEO","tokenCount":1000}]}`
	// Half price at 04:00 UTC: (100 x 0.1875) + (1000 x 0.25) = 268.75 quota.
	_, got := settleLiveReceiptWithTariff(t, persistedVertexLiveTariff(t, strings.Replace(window, "%s", "", 1)), avatar)
	require.False(t, got.UnpricedUsage, "%v", got.BillingIssues)
	require.EqualValues(t, 269, got.TotalQuota)
	// The overlay doubles the video ratio: 18.75 + 1000 x 0.1875 x 2.6666... = 518.75.
	overlay := strings.Replace(window, "%s", `, "video": {"output_token_ratio": 2.6666666666666665}`, 1)
	_, got = settleLiveReceiptWithTariff(t, persistedVertexLiveTariff(t, overlay), avatar)
	require.False(t, got.UnpricedUsage, "%v", got.BillingIssues)
	require.EqualValues(t, 519, got.TotalQuota)
}
