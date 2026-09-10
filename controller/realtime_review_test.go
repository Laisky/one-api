package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestReviewMissingFinalReceiptIsNotIdle exercises the fallback decision used by
// production settlement with disconnects, true idle and authoritative zero usage.
func TestReviewMissingFinalReceiptIsNotIdle(t *testing.T) {
	for _, tc := range []struct {
		name      string
		frames    []string
		estimated bool
	}{
		{"idle", nil, false},
		{"authoritative_zero", []string{`{"type":"response.done","response":{"id":"r","usage":{"input_tokens":0,"output_tokens":0}}}`}, false},
		{"disconnect_after_created", []string{`{"type":"response.created","response":{"id":"r"}}`}, true},
		{"missing_usage", []string{`{"type":"response.done","response":{"id":"r","usage":null}}`}, true},
		{"invalid_usage", []string{`{"type":"response.done","response":{"id":"r","usage":{"input_tokens":-1,"output_tokens":0}}}`}, true},
		{"partial_then_disconnect", []string{`{"type":"response.done","response":{"id":"r1","usage":{"input_tokens":10,"output_tokens":0}}}`, `{"type":"response.created","response":{"id":"r2"}}`}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := realtime.NewLedger()
			for _, frame := range tc.frames {
				_ = ledger.Observe([]byte(frame))
			}
			ledger.Finish()
			usage := &relaymodel.Usage{Realtime: ledger}
			require.Equal(t, tc.estimated, retainRealtimeEstimate(usage), "unknown final usage must not take the idle-refund branch")
			result := computeRealtimeSessionQuota(quota.ComputeInput{ModelName: "gpt-realtime", ModelRatio: 2,
				GroupRatio: 1, PricingAdaptor: &openai.Adaptor{}, Usage: usage}, nil)
			metadata := realtimeReceiptMetadata(usage, result)
			require.Equal(t, !tc.estimated, metadata["realtime_billing_complete"])
		})
	}
}

// TestReviewReceiptMetadataIsBounded serializes what settlement actually stores,
// rather than measuring the ledger alone. MySQL TEXT allows at most 65535 bytes.
func TestReviewReceiptMetadataIsBounded(t *testing.T) {
	ledger := realtime.NewLedger()
	for i := 0; i < 1000; i++ {
		frame := fmt.Sprintf(`{"type":"response.done","response":{"id":"%s%d","usage":{"input_tokens":1,"output_tokens":0}}}`, strings.Repeat("r", 120), i)
		require.NoError(t, ledger.Observe([]byte(frame)))
	}
	ledger.Finish()
	usage := &relaymodel.Usage{Realtime: ledger}
	result := computeRealtimeSessionQuota(quota.ComputeInput{ModelName: "gpt-realtime", ModelRatio: 2,
		GroupRatio: 1, PricingAdaptor: &openai.Adaptor{}, Usage: usage}, nil)
	require.Equal(t, int64(2000), result.TotalQuota, "metadata bounding must not discard billable receipts")
	encoded, err := json.Marshal(realtimeReceiptMetadata(usage, result))
	require.NoError(t, err)
	require.Less(t, len(encoded), 32*1024, "leave room for other log metadata within TEXT")
}
