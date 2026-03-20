package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	rmodel "github.com/songquanpeng/one-api/relay/model"
)

// TestMaybeParseRealtimeUsage verifies basic usage extraction from realtime events.
func TestMaybeParseRealtimeUsage(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{"type":"response.done","response":{"id":"resp_rt_1","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}}`)
	maybeParseRealtimeUsage(msg, usage, counted)

	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
}

// TestMaybeParseRealtimeUsageDeduplicate verifies that repeated events for the
// same response ID are only counted once.
func TestMaybeParseRealtimeUsageDeduplicate(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{"type":"response.done","response":{"id":"resp_rt_dedup","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}}`)

	maybeParseRealtimeUsage(msg, usage, counted)
	maybeParseRealtimeUsage(msg, usage, counted) // duplicate

	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
	require.Len(t, counted, 1)
}

// TestMaybeParseRealtimeUsageMultipleResponses verifies usage from different
// response IDs are accumulated correctly.
func TestMaybeParseRealtimeUsageMultipleResponses(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg1 := []byte(`{"type":"response.done","response":{"id":"resp_rt_a","usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}}`)
	msg2 := []byte(`{"type":"response.done","response":{"id":"resp_rt_b","usage":{"input_tokens":5,"output_tokens":15,"total_tokens":20}}}`)

	maybeParseRealtimeUsage(msg1, usage, counted)
	maybeParseRealtimeUsage(msg2, usage, counted)

	require.Equal(t, 15, usage.PromptTokens)
	require.Equal(t, 35, usage.CompletionTokens)
	require.Equal(t, 50, usage.TotalTokens)
	require.Len(t, counted, 2)
}

// TestMaybeParseRealtimeUsageNilSafety verifies nil inputs don't panic.
func TestMaybeParseRealtimeUsageNilSafety(t *testing.T) {
	t.Parallel()

	counted := map[string]struct{}{}
	// nil usage
	maybeParseRealtimeUsage([]byte(`{"response":{"id":"r","usage":{"input_tokens":1}}}`), nil, counted)

	// empty msg
	usage := &rmodel.Usage{}
	maybeParseRealtimeUsage(nil, usage, counted)
	maybeParseRealtimeUsage([]byte{}, usage, counted)

	// invalid json
	maybeParseRealtimeUsage([]byte(`{invalid`), usage, counted)

	require.Equal(t, 0, usage.PromptTokens)
}

// TestMaybeParseRealtimeUsageNoResponse verifies events without response object are ignored.
func TestMaybeParseRealtimeUsageNoResponse(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{"type":"session.created","session":{"id":"sess_1"}}`)
	maybeParseRealtimeUsage(msg, usage, counted)

	require.Equal(t, 0, usage.PromptTokens)
	require.Equal(t, 0, usage.CompletionTokens)
}

// TestMaybeParseRealtimeUsageNoResponseID verifies events without response ID
// are still counted (backward compatibility) but won't be deduplicated.
func TestMaybeParseRealtimeUsageNoResponseID(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	msg := []byte(`{"type":"response.done","response":{"usage":{"input_tokens":10,"output_tokens":20}}}`)
	maybeParseRealtimeUsage(msg, usage, counted)
	maybeParseRealtimeUsage(msg, usage, counted)

	// Without response ID, deduplication doesn't apply, so tokens are counted twice
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 40, usage.CompletionTokens)
}

// TestCopyWSRealtimeUsageFullEvent verifies a realistic realtime response.done
// event with nested usage is parsed correctly.
func TestCopyWSRealtimeUsageFullEvent(t *testing.T) {
	t.Parallel()

	fullEvent := map[string]any{
		"type":     "response.done",
		"event_id": "event_abc",
		"response": map[string]any{
			"id":     "resp_full_1",
			"object": "realtime.response",
			"status": "completed",
			"usage": map[string]any{
				"input_tokens":  150,
				"output_tokens": 75,
				"total_tokens":  225,
			},
		},
	}
	msg, _ := json.Marshal(fullEvent)

	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}
	maybeParseRealtimeUsage(msg, usage, counted)

	require.Equal(t, 150, usage.PromptTokens)
	require.Equal(t, 75, usage.CompletionTokens)
	require.Equal(t, 225, usage.TotalTokens)
}
