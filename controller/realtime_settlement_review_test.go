package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestReviewSettlementPreservesEvidence exercises the exact charge/metadata
// preparation called by postConsumeRealtimeQuota, including refund controls.
func TestReviewSettlementPreservesEvidence(t *testing.T) {
	const pending = `{"type":"response.created","response":{"id":"pending"}}`
	const valid = `{"type":"response.done","response":{"id":"paid","usage":{"input_tokens":10,"output_tokens":0}}}`
	const missing = `{"type":"response.done","response":{"id":"paid","usage":null}}`
	for _, tc := range []struct {
		name, model string
		frames      []string
		group       float64
		want        int64
		estimated   bool
	}{
		{"idle", "gpt-realtime", nil, 1, 0, false},
		{"explicit_zero", "gpt-realtime", []string{`{"type":"response.done","response":{"id":"zero","usage":{"input_tokens":0,"output_tokens":0}}}`}, 1, 0, false},
		{"missing_final", "gpt-realtime", []string{pending}, 1, 1234, true},
		{"missing_usage", "gpt-realtime", []string{missing}, 1, 1234, true},
		{"known_below_reservation_and_gap", "gpt-realtime", []string{valid, pending}, 1, 1234, true},
		{"known_above_reservation_and_gap", "gpt-realtime", []string{`{"type":"response.done","response":{"id":"large","usage":{"input_tokens":1000,"output_tokens":0}}}`, pending}, 1, 2000, true},
		{"corrected_receipt", "gpt-realtime", []string{missing, valid}, 1, 20, false},
		{"normal_short_session", "gpt-realtime", []string{valid}, 1, 20, false},
		{"free_group_with_gap", "gpt-realtime", []string{valid, pending}, 0, 0, false},
		{"unpriceable_model", "unknown-review-model", []string{valid}, 1, 1234, true},
		{"duration_only", "gpt-realtime", []string{
			`{"type":"session.updated","session":{"input_audio_transcription":{"model":"whisper-1"}}}`,
			`{"type":"input_audio_buffer.committed","item_id":"audio"}`,
			`{"type":"conversation.item.input_audio_transcription.completed","item_id":"audio","content_index":0,"usage":{"type":"duration","seconds":12.5}}`,
		}, 1, 625, false},
		{"rejected_missing_index_then_valid", "gpt-realtime", []string{
			`{"type":"session.updated","session":{"input_audio_transcription":{"model":"whisper-1"}}}`,
			`{"type":"input_audio_buffer.committed","item_id":"audio"}`,
			`{"type":"conversation.item.input_audio_transcription.completed","item_id":"audio","usage":{"type":"duration","seconds":100}}`,
			`{"type":"conversation.item.input_audio_transcription.completed","item_id":"audio","content_index":0,"usage":{"type":"duration","seconds":1}}`,
		}, 1, 50, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := realtime.NewLedger()
			for _, frame := range tc.frames {
				_ = ledger.Observe([]byte(frame))
			}
			ledger.Finish()
			input := quota.ComputeInput{ModelName: tc.model, ModelRatio: 2, GroupRatio: tc.group,
				PricingAdaptor: &openai.Adaptor{}, Usage: &rmodel.Usage{Realtime: ledger}}
			got, metadata := prepareRealtimeReceiptSettlement(input, 1234, nil)
			require.Equal(t, tc.want, got.TotalQuota)
			estimated, _ := metadata[model.LogMetadataKeyEstimatedCharge].(bool)
			require.Equal(t, tc.estimated, estimated)
			if estimated {
				require.Equal(t, false, metadata["realtime_billing_complete"])
				require.Contains(t, metadata, "realtime_observed_quota")
			}
			again, _ := prepareRealtimeReceiptSettlement(input, 1234, nil)
			require.Equal(t, got.TotalQuota, again.TotalQuota)
			require.Zero(t, input.Usage.ToolsCost, "settlement must not accumulate a second audio surcharge")
		})
	}
}

// TestReviewAuditEscapingAndIsolation bounds encoded bytes even for worst-case
// JSON escaping and proves the persisted snapshot cannot mutate live evidence.
func TestReviewAuditEscapingAndIsolation(t *testing.T) {
	ledger := realtime.NewLedger()
	ledger.Records = []realtime.Record{{Model: strings.Repeat("\x00", 256), Duration: true, Seconds: 1}}
	for i := 0; i < realtime.MaxIssues; i++ {
		ledger.Issues = append(ledger.Issues, strings.Repeat("\x00", 256))
	}
	usage := &rmodel.Usage{Realtime: ledger}
	metadata := realtimeReceiptMetadata(usage, quota.ComputeResult{BillingIssues: ledger.Issues})
	encoded, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.Less(t, len(encoded), 32*1024)
	require.Equal(t, true, metadata["realtime_audit_detail_truncated"])
	audit := metadata["realtime_usage"].(realtime.AuditSnapshot)
	require.Equal(t, 1, audit.ReceiptCount)
	require.Len(t, audit.RecordsSHA256, 64)
	require.True(t, audit.SampleTruncated)
	ledger.Records = []realtime.Record{{Tokens: realtime.Tokens{Input: 1, Text: 1}}}
	ledger.Issues = nil
	metadata = realtimeReceiptMetadata(usage, quota.ComputeResult{})
	audit = metadata["realtime_usage"].(realtime.AuditSnapshot)
	ledger.Records[0].Tokens.Text = 999
	require.Equal(t, int64(1), audit.SampleRecords[0].Tokens.Text)
}
