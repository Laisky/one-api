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

// vertexLive38OperatorTariff returns the operator tariff that reproduces the
// Vertex AI pricing page row "Gemini 3.8 Live API", Non-global
// (https://cloud.google.com/vertex-ai/generative-ai/pricing, fetched
// 2026-11-18T14:34Z): text $0.75 in / $4.50 out, audio $3.00 in / $12.00 out,
// video/image $1.00 in, and "Output: video (avatar)" $1.00 per 1M tokens. It
// deliberately omits video.output_token_ratio, the field that prices the
// video-output row, to pin the behaviour of tariffs written before that field
// existed. It takes no arguments and returns the channel model-config map for
// gemini-3.8-live.
func vertexLive38OperatorTariff() map[string]modelcfg.ModelConfigLocal {
	return map[string]modelcfg.ModelConfigLocal{"gemini-3.8-live": {
		Ratio:           0.75 * ratio.MilliTokensUsd,
		CompletionRatio: 4.50 / 0.75,
		Audio:           &modelcfg.AudioPricingLocal{PromptRatio: 3.00 / 0.75, CompletionRatio: 12.00 / 3.00},
		Image:           &modelcfg.ImagePricingLocal{PromptRatio: 1.00 / 0.75},
	}}
}

// settleVertexLiveReceipt decodes raw as a Gemini Live usageMetadata receipt and
// settles it through quota.Compute under the Vertex adaptor and the operator
// tariff above. It accepts the test handle and the raw receipt and returns the
// decoded record and the compute result.
func settleVertexLiveReceipt(t *testing.T, raw string) (realtime.Record, quota.ComputeResult) {
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
		ChannelModelConfigs: vertexLive38OperatorTariff(),
		PricingAdaptor:      &vertexai.Adaptor{},
		RequestTime:         time.Date(2026, 11, 18, 4, 0, 0, 0, time.UTC),
	})
}

// TestVertexLiveAvatarVideoOutputIsNotSilentlyPriced settles a Vertex Live
// receipt whose response is attributed to the VIDEO modality (avatar output).
// The operator tariff sets no video output price, so the receipt must be
// flagged as unpriced with a visible billing issue instead of settling as
// exact, and it must never be charged below the published $1.00/1M avatar rate.
// A text/audio-only receipt in the same session stays exactly priced. It
// accepts t and returns no values.
func TestVertexLiveAvatarVideoOutputIsNotSilentlyPriced(t *testing.T) {
	t.Parallel()
	const avatar = `{"promptTokenCount":100,"responseTokenCount":1000,"totalTokenCount":1100,` +
		`"promptTokensDetails":[{"modality":"TEXT","tokenCount":100}],` +
		`"responseTokensDetails":[{"modality":"VIDEO","tokenCount":1000}]}`
	record, got := settleVertexLiveReceipt(t, avatar)
	require.EqualValues(t, 1000, record.Tokens.OutputVideo, "the receipt keeps Google's explicit VIDEO attribution")
	require.True(t, got.UnpricedUsage, "video output has no configurable price; it must not settle as exact billing")
	joined := strings.Join(got.BillingIssues, "; ")
	require.Contains(t, joined, "video output", "the unpriced dimension must be named in billing issues")
	// Published floor: 100 text-in tokens at $0.75/1M plus 1000 avatar tokens at
	// $1.00/1M = $0.001075 = 537.5 quota at 500000 quota/USD.
	require.EqualValues(t, 500000, ratio.QuotaPerUsd, "quota scale assumption")
	require.GreaterOrEqual(t, got.TotalQuota, int64(538), "the receipt is never charged below the published avatar rate")
	// Unchanged legacy charge: the avatar tokens stay unallocated output at the
	// cheaper of text ($4.50) and audio ($12.00) output, 37.5 + 2250 quota.
	require.EqualValues(t, 2288, got.TotalQuota)

	const plain = `{"promptTokenCount":100,"responseTokenCount":1000,"totalTokenCount":1100,` +
		`"promptTokensDetails":[{"modality":"TEXT","tokenCount":100}],` +
		`"responseTokensDetails":[{"modality":"TEXT","tokenCount":400},{"modality":"AUDIO","tokenCount":600}]}`
	record, got = settleVertexLiveReceipt(t, plain)
	require.Zero(t, record.Tokens.OutputVideo)
	require.False(t, got.UnpricedUsage, "text/audio receipts remain exactly priced: %v", got.BillingIssues)
	require.Empty(t, got.BillingIssues)
	// 100 x 0.75 + 400 x 4.50 + 600 x 12.00 = $9.075 per 1M = 4537.5 quota.
	require.EqualValues(t, 4538, got.TotalQuota)
}
