package openai

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	rmodel "github.com/Laisky/one-api/relay/model"
)

// TestResponseWSTerminalGateRejectsProvisional checks existing extraction,
// deduplication and ownership APIs against independent nonterminal JSON frames.
func TestResponseWSTerminalGateRejectsProvisional(t *testing.T) {
	for _, test := range []struct {
		name, event, status string
	}{
		{"progress", "response.in_progress", "in_progress"},
		{"created", "response.created", "queued"},
		{"progress_with_completed_status", "response.in_progress", "completed"},
		{"unrelated_event_with_completed_status", "response.output_text.delta", "completed"},
		{"completed_with_queued_status", "response.completed", "queued"},
		{"completed_with_progress_status", "response.completed", "in_progress"},
		{"completed_with_unknown_status", "response.completed", "future_state"},
	} {
		payload := []byte(fmt.Sprintf(`{"type":%q,"response":{"id":"resp_gate","status":%q,"usage":{"input_tokens":3,"output_tokens":0,"total_tokens":3}}}`, test.event, test.status))
		t.Run(test.name+"/extract", func(t *testing.T) {
			id, usage, ok := extractResponseAPIUsage(payload)
			require.False(t, ok, "provisional event/status cannot be an authoritative receipt")
			require.Empty(t, id)
			require.Nil(t, usage)
		})
		t.Run(test.name+"/deduplication", func(t *testing.T) {
			usage := &rmodel.Usage{}
			counted := map[string]struct{}{}
			accumulateResponseAPIUsage(payload, usage, counted)
			require.Empty(t, counted, "a provisional snapshot must not claim the terminal receipt ID")
			require.Zero(t, usage.PromptTokens)
			require.Zero(t, usage.CompletionTokens)
			require.Zero(t, usage.TotalTokens)
		})
		t.Run(test.name+"/ownership", func(t *testing.T) {
			ownership := &responseWSSessionOwnership{}
			collector := &responseAPIWSStoreCollector{ownership: ownership}
			collector.collect(payload)
			require.Empty(t, ownership.ids, "unfinished response IDs must not authorize a continuation")
			require.Empty(t, collector.responses)
			require.Empty(t, collector.seen)
		})
	}
}

// TestResponseWSTerminalGateLaterReceipt keeps the final receipt authoritative
// after provisional same-ID usage and counts duplicate terminal frames once.
func TestResponseWSTerminalGateLaterReceipt(t *testing.T) {
	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}
	accumulateResponseAPIUsage([]byte(`{"type":"response.in_progress","response":{"id":"resp_later","status":"in_progress","usage":{"input_tokens":3,"output_tokens":0,"total_tokens":3}}}`), usage, counted)
	terminal := []byte(`{"type":"response.completed","response":{"id":"resp_later","status":"completed","usage":{"input_tokens":3,"output_tokens":100,"total_tokens":103}}}`)
	accumulateResponseAPIUsage(terminal, usage, counted)
	accumulateResponseAPIUsage(terminal, usage, counted)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 103, usage.TotalTokens)
	require.Len(t, counted, 1)
	accumulateResponseAPIUsage([]byte(`{"type":"response.completed","response":{"id":"resp_independent","usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`), usage, counted)
	require.Equal(t, 8, usage.PromptTokens)
	require.Equal(t, 102, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
	require.Len(t, counted, 2)
}

// TestResponseWSTerminalGateReceiptControls preserves known terminal types,
// status-omitted legacy receipts and the existing bridge status compatibility.
func TestResponseWSTerminalGateReceiptControls(t *testing.T) {
	for _, test := range []struct {
		name, event, status string
	}{
		{"completed", "response.completed", `,"status":"completed"`},
		{"failed", "response.failed", `,"status":"failed"`},
		{"incomplete", "response.incomplete", `,"status":"incomplete"`},
		{"completed_status_omitted", "response.completed", ""},
		{"failed_status_omitted", "response.failed", ""},
		{"incomplete_status_omitted", "response.incomplete", ""},
		{"bridge_failed", "response.completed", `,"status":"failed"`},
		{"bridge_incomplete", "response.completed", `,"status":"incomplete"`},
		{"cancelled_status_compatibility", "response.completed", `,"status":"cancelled"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := []byte(fmt.Sprintf(`{"type":%q,"response":{"id":"resp_control"%s,"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18,"cache_write_tokens":2,"cache_write_5m_tokens":3,"cache_write_1h_tokens":4,"input_tokens_details":{"cached_tokens":5},"output_tokens_details":{"reasoning_tokens":6}}}}`, test.event, test.status))
			id, usage, ok := extractResponseAPIUsage(payload)
			require.True(t, ok)
			require.Equal(t, "resp_control", id)
			require.NotNil(t, usage)
			require.Equal(t, 11, usage.PromptTokens)
			require.Equal(t, 7, usage.CompletionTokens)
			require.Equal(t, 18, usage.TotalTokens)
			require.Equal(t, 5, usage.CacheWrite5mTokens)
			require.Equal(t, 4, usage.CacheWrite1hTokens)
			require.NotNil(t, usage.PromptTokensDetails)
			require.Equal(t, 5, usage.PromptTokensDetails.CachedTokens)
			require.NotNil(t, usage.CompletionTokensDetails)
			require.Equal(t, 6, usage.CompletionTokensDetails.ReasoningTokens)
			aggregate := &rmodel.Usage{}
			counted := map[string]struct{}{}
			accumulateResponseAPIUsage(payload, aggregate, counted)
			accumulateResponseAPIUsage(payload, aggregate, counted)
			require.Equal(t, usage, aggregate, "duplicates cannot duplicate any measured cache bucket")
			require.Len(t, counted, 1)
		})
	}
}

// TestResponseWSTerminalGateOwnershipControls preserves completed-envelope
// ownership, store=false connection locality and status-omitted compatibility.
func TestResponseWSTerminalGateOwnershipControls(t *testing.T) {
	for _, test := range []struct {
		name, event, status string
		owned               bool
	}{
		{"completed", "response.completed", `,"status":"completed"`, true},
		{"omitted", "response.completed", "", true},
		{"bridge_failed", "response.completed", `,"status":"failed"`, true},
		{"bridge_incomplete", "response.completed", `,"status":"incomplete"`, true},
		{"cancelled_compatibility", "response.completed", `,"status":"cancelled"`, true},
		{"failed_envelope", "response.failed", `,"status":"failed"`, false},
		{"incomplete_envelope", "response.incomplete", `,"status":"incomplete"`, false},
	} {
		for _, store := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/store_%v", test.name, store), func(t *testing.T) {
				payload := []byte(fmt.Sprintf(`{"type":%q,"response":{"id":"resp_owned"%s,"store":%v,"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`, test.event, test.status, store))
				ownership := &responseWSSessionOwnership{}
				collector := &responseAPIWSStoreCollector{ownership: ownership}
				collector.collect(payload)
				collector.collect(payload)
				if !test.owned {
					require.Empty(t, ownership.ids)
					require.Empty(t, collector.responses)
					return
				}
				require.Len(t, ownership.ids, 1)
				require.Contains(t, ownership.ids, "resp_owned")
				if store {
					require.Len(t, collector.responses, 1)
					require.Equal(t, "resp_owned", collector.responses[0].Id)
					require.Len(t, collector.seen, 1)
				} else {
					require.Empty(t, collector.responses)
					require.Empty(t, collector.seen)
				}
			})
		}
	}
}
