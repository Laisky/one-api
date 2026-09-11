package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestRealtimeReceiptSettlementBoundary ensures the production controller does
// not apply its legacy full-price audio surcharge on top of receipt pricing.
func TestRealtimeReceiptSettlementBoundary(t *testing.T) {
	t.Parallel()
	ledger := realtime.NewLedger()
	require.NoError(t, ledger.Observe([]byte(`{"type":"response.done","response":{"id":"cached","usage":{"input_tokens":1000000,"output_tokens":0,"input_token_details":{"audio_tokens":1000000,"cached_tokens":1000000,"cached_tokens_details":{"audio_tokens":1000000}}}}}`)))
	usage := &rmodel.Usage{Realtime: ledger, PromptTokens: 1000000,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{AudioTokens: 1000000, CachedTokens: 1000000}}
	input := quota.ComputeInput{Usage: usage, ModelName: "gpt-realtime", ModelRatio: 2, GroupRatio: 1, PricingAdaptor: &openai.Adaptor{}}
	result := computeRealtimeSessionQuota(input, nil)
	require.Empty(t, result.BillingIssues)
	require.Equal(t, int64(200000), result.TotalQuota) // $0.40/1M, not $28.40/1M
	require.Zero(t, usage.ToolsCost)
	require.Equal(t, result, computeRealtimeSessionQuota(input, nil))
	require.True(t, realtimeReceiptMetadata(usage, result)["realtime_billing_complete"].(bool))
	result.BillingIssues = []string{"missing response receipt"}
	require.False(t, realtimeReceiptMetadata(usage, result)["realtime_billing_complete"].(bool))
}

// TestRealtimeReceiptZeroTokenSettlement preserves legacy behavior without
// treating idle connections or duration-billed transcription as missing usage.
func TestRealtimeReceiptZeroTokenSettlement(t *testing.T) {
	t.Parallel()
	require.True(t, retainRealtimeEstimate(nil))
	require.True(t, retainRealtimeEstimate(&rmodel.Usage{}))
	ledger := realtime.NewLedger()
	usage := &rmodel.Usage{Realtime: ledger}
	require.False(t, retainRealtimeEstimate(usage))
	input := quota.ComputeInput{Usage: usage, ModelName: "gpt-realtime", ModelRatio: 2, GroupRatio: 1, PricingAdaptor: &openai.Adaptor{}}
	require.Zero(t, computeRealtimeSessionQuota(input, nil).TotalQuota)
	ledger.Records = []realtime.Record{{Model: "whisper-1", Duration: true, Seconds: 12.5}}
	result := computeRealtimeSessionQuota(input, nil)
	require.Empty(t, result.BillingIssues)
	require.Equal(t, int64(625), result.TotalQuota)
	require.Zero(t, result.PromptTokens+result.CompletionTokens)
	require.False(t, retainRealtimeEstimate(usage))
}
