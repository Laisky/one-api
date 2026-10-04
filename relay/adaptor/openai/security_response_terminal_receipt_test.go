package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurityResponseTerminalReceiptClassification keeps provider finality
// separate from transport framing and synthetic item-completion conversion.
func TestSecurityResponseTerminalReceiptClassification(t *testing.T) {
	const receipt = `"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}`
	cases := []struct {
		name     string
		payload  string
		terminal bool
	}{
		{"legacy_final", `{"id":"resp_fixture",` + receipt + `}`, true},
		{"full_completed", `{"id":"resp_fixture","status":"completed",` + receipt + `}`, true},
		{"full_failed", `{"id":"resp_fixture","status":"failed",` + receipt + `}`, true},
		{"full_incomplete", `{"id":"resp_fixture","status":"incomplete",` + receipt + `}`, true},
		{"full_queued", `{"id":"resp_fixture","status":"queued",` + receipt + `}`, false},
		{"full_in_progress", `{"id":"resp_fixture","status":"in_progress",` + receipt + `}`, false},
		{"full_unknown", `{"id":"resp_fixture","status":"future_state",` + receipt + `}`, false},
		{"missing_receipt", `{"id":"resp_fixture","status":"completed"}`, false},
		{"typed_final_without_status", `{"type":"response.completed","response":{"id":"resp_fixture",` + receipt + `}}`, true},
		{"typed_failed_without_status", `{"type":"response.failed","response":{"id":"resp_fixture",` + receipt + `}}`, true},
		{"typed_incomplete_without_status", `{"type":"response.incomplete","response":{"id":"resp_fixture",` + receipt + `}}`, true},
		{"typed_final_missing_receipt", `{"type":"response.completed","response":{"id":"resp_fixture"}}`, false},
		{"typed_progress", `{"type":"response.in_progress","response":{"id":"resp_fixture","status":"in_progress",` + receipt + `}}`, false},
		{"contradictory_status", `{"type":"response.completed","response":{"id":"resp_fixture","status":"in_progress",` + receipt + `}}`, false},
		{"item_completion_is_not_response_completion", `{"type":"response.output_json.done","json":{"ok":true},` + receipt + `}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full, event, err := ParseResponseAPIStreamEvent([]byte(tc.payload))
			require.NoError(t, err)
			var normalized ResponseAPIResponse
			if full != nil {
				normalized = *full
			} else {
				require.NotNil(t, event)
				normalized = ConvertStreamEventToResponse(event)
			}
			require.Equal(t, tc.terminal, responseStreamHasTerminalUsage(full, event, &normalized))
		})
	}
	require.False(t, responseStreamHasTerminalUsage(nil, nil, nil))
}
