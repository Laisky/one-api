package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	rmodel "github.com/Laisky/one-api/relay/model"
)

// TestResponseWSNoIDReceiptFinalization preserves explicit-zero and duplicate receipt authority while labeling successful creations without terminal evidence.
func TestResponseWSNoIDReceiptFinalization(t *testing.T) {
	zero := []byte(`{"type":"response.completed","response":{"id":"resp_zero","status":"completed","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`)
	other := []byte(`{"type":"response.completed","response":{"id":"resp_other","status":"completed","usage":{"input_tokens":3,"output_tokens":100,"total_tokens":103}}}`)
	progress := []byte(`{"type":"response.in_progress","response":{"id":"resp_pending","status":"in_progress","usage":{"input_tokens":3,"output_tokens":0,"total_tokens":3}}}`)
	for _, tc := range []struct {
		name       string
		dispatched int64
		events     [][]byte
		counted    int
		estimated  bool
	}{
		{name: "no_dispatch"},
		{name: "first_create_without_id", dispatched: 1, estimated: true},
		{name: "terminal_then_create_without_id", dispatched: 2, events: [][]byte{other}, counted: 1, estimated: true},
		{name: "explicit_zero_terminal", dispatched: 1, events: [][]byte{zero}, counted: 1},
		{name: "duplicate_zero_terminal", dispatched: 1, events: [][]byte{zero, zero}, counted: 1},
		{name: "duplicate_id_cannot_settle_second_create", dispatched: 2, events: [][]byte{zero, zero}, counted: 1, estimated: true},
		{name: "two_distinct_terminal_receipts", dispatched: 2, events: [][]byte{zero, other}, counted: 2},
		{name: "pending_id_survives_other_terminal", dispatched: 1, events: [][]byte{progress, other}, counted: 1, estimated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := &rmodel.Usage{}
			collector := &responseAPIWSUsageCollector{usage: usage}
			for _, event := range tc.events {
				collector.collect(event)
			}
			collector.finish(tc.dispatched)
			require.Len(t, collector.counted, tc.counted)
			if tc.estimated {
				require.Equal(t, "response_stream_incomplete_or_missing_receipt", usage.BillingEstimateReason)
			} else {
				require.Empty(t, usage.BillingEstimateReason)
			}
		})
	}
}
