package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseUsageJSONReviewBoundary retains independent JSON byte oracles for raw deltas, application keys, and separate items.
func TestResponseUsageJSONReviewBoundary(t *testing.T) {
	payload := `{"answer":"` + strings.Repeat("synthetic accounting evidence ", 100) + `"}`
	app := `{"text":"x","answer":"` + strings.Repeat("synthetic accounting evidence ", 100) + `"}`
	q, err := json.Marshal(payload)
	require.NoError(t, err)
	whitespace := `{"answer":` + strings.Repeat(" ", 2000) + `"synthetic"}`
	cases := []struct {
		name, body string
		expected   int
	}{
		{"plain_object_whitespace", "data: " + `{"type":"response.output_json.delta","output_index":0,"delta":` + whitespace + `}` + "\n\n", len(whitespace)},
		{"late_identity_alias", "data: " + `{"type":"response.output_json.delta","item_id":"a","delta":"{\"a\":"}` + "\n\n" + "data: " + `{"type":"response.output_json.delta","output_index":0,"delta":"1}"}` + "\n\n" + "data: " + `{"type":"response.output_json.done","item_id":"a","output_index":0,"json":{"a":1}}` + "\n\n", 7},
		{"plain_object_delta", "data: " + `{"type":"response.output_json.delta","output_index":0,"delta":` + payload + `}` + "\n\n", len(payload)},
		{"part_application_text", "data: " + `{"type":"response.output_json.done","output_index":0,"part":{"type":"output_json","json":` + app + `}}` + "\n\n", len(app)},
		{"distinct_items", "data: " + `{"type":"response.output_json.delta","item_id":"a","output_index":0,"delta":"{\"a\":1}"}` + "\n\n" + "data: " + `{"type":"response.output_json.done","item_id":"b","output_index":1,"json":{"b":2}}` + "\n\n", 14},
		{"same_item_snapshot_control", "data: " + `{"type":"response.output_json.delta","item_id":"a","output_index":0,"delta":{"partial_json":` + string(q) + `}}` + "\n\n" + "data: " + `{"type":"response.output_json.done","item_id":"a","output_index":0,"json":` + payload + `}` + "\n\n", len(payload)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usage := ParseResponseUsage([]byte(tc.body), 7, func(text string) int { return len(text) })
			require.Equal(t, tc.expected, usage.CompletionTokens)
			require.Equal(t, 7+tc.expected, usage.TotalTokens)
		})
	}
}
