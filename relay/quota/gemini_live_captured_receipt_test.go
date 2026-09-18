package quota_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestGeminiLiveCapturedReceiptSettles prices a receipt captured from a real
// gemini-3.8-live conversation on 2026-09-18. Parameters: t is the test handle.
// Returns: none. Live receipts do not close their modality breakdown and report
// thinking tokens outside the aggregate; this asserts the whole turn is billed
// rather than discarded, and that the unexplained remainder costs no more than
// the cheapest input modality.
func TestGeminiLiveCapturedReceiptSettles(t *testing.T) {
	t.Parallel()
	const captured = `{"promptTokenCount":548,"promptTokensDetails":[{"modality":"TEXT","tokenCount":306},{"modality":"AUDIO","tokenCount":222}],"responseTokenCount":61,"responseTokensDetails":[{"modality":"AUDIO","tokenCount":61}],"thoughtsTokenCount":100,"totalTokenCount":609}`
	record, err := realtime.DecodeGeminiUsage([]byte(captured))
	require.NoError(t, err)

	ledger := realtime.NewLedger()
	ledger.Records = []realtime.Record{record}
	ledger.InputTokens, ledger.OutputTokens = record.Tokens.Input, record.Tokens.Output
	result := quota.Compute(quota.ComputeInput{
		Usage:          &model.Usage{Realtime: ledger},
		ModelName:      "gemini-3.8-live",
		ModelRatio:     0.75 * ratio.MilliTokensUsd,
		GroupRatio:     1,
		PricingAdaptor: &gemini.Adaptor{},
		RequestTime:    time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC),
	})

	// USD per million: text in 0.75, audio in 3, text out 4.50, audio out 12. The
	// 20 unattributed input tokens are charged at the cheapest input modality.
	// (306*0.75 + 222*3 + 20*0.75 + 100*4.50 + 61*12)/1e6 USD * 500000 quota/USD
	// = 1046.25 quota, rounded up once for the session.
	require.False(t, result.UnpricedUsage)
	require.Empty(t, result.BillingIssues)
	require.EqualValues(t, 548, result.PromptTokens)
	require.EqualValues(t, 161, result.CompletionTokens, "thinking tokens the aggregate omits are still billable output")
	require.EqualValues(t, 1047, result.TotalQuota, "session quota rounds up once")
	require.EqualValues(t, 500000, ratio.QuotaPerUsd, "the expected quota above assumes this scale")
}
