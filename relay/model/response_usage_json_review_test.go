package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseUsageJSONEvidence counts structured output and object arguments while keeping snapshots separate from deltas.
func TestResponseUsageJSONEvidence(t *testing.T) {
	payload := `{"answer":"` + strings.Repeat("synthetic accounting evidence ", 100) + `"}`
	quoted, err := json.Marshal(payload)
	require.NoError(t, err)
	cases := []struct {
		name, body string
		expected   int
	}{
		{"json_delta_and_done", "data: " + `{"type":"response.output_json.delta","output_index":0,"content_index":0,"delta":{"partial_json":` + string(quoted) + `}}` + "\n\n" + "data: " + `{"type":"response.output_json.done","output_index":0,"content_index":0,"json":` + payload + `}` + "\n\n", len(payload)},
		{"json_done_only", "data: " + `{"type":"response.output_json.done","json":` + payload + `}` + "\n\n", len(payload)},
		{"output_json_block", `{"output":[{"type":"message","content":[{"type":"output_json","json":` + payload + `}]}]}`, len(payload)},
		{"chat_object_tool_arguments", `{"choices":[{"message":{"tool_calls":[{"function":{"name":"lookup","arguments":` + payload + `}}]}}]}`, len("lookup") + len(payload)},
		{"legacy_object_arguments", `{"choices":[{"message":{"function_call":{"name":"lookup","arguments":` + payload + `}}}]}`, len("lookup") + len(payload)},
		{"response_object_arguments", `{"output":[{"type":"function_call","name":"lookup","arguments":` + payload + `}]}`, len("lookup") + len(payload)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acc := NewResponseUsageAccumulator(false)
			acc.Observe([]byte(tc.body))
			usage := acc.Finish(7, func(text string) int { return len(text) })
			require.Equal(t, tc.expected, usage.CompletionTokens, "delivered JSON is billable once")
			require.Equal(t, 7+tc.expected, usage.TotalTokens)
			require.NotEmpty(t, usage.BillingEstimateReason)
			require.Equal(t, usage, acc.Finish(7, func(text string) int { return len(text) }))
		})
	}
}

// TestResponseUsageJSONMeasuredControl preserves explicit usage even when structured output is much larger.
func TestResponseUsageJSONMeasuredControl(t *testing.T) {
	body := `{"output":[{"type":"message","content":[{"type":"output_json","json":{"answer":"` + strings.Repeat("synthetic ", 100) + `"}}]}],"usage":{"input_tokens":7,"output_tokens":3}}`
	usage := ParseResponseUsage([]byte(body), 99, func(text string) int { return len(text) })
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Empty(t, usage.BillingEstimateReason)
}
