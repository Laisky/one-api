package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseUsageInterruptionAfterTerminalReceipt verifies that a transport
// failure only marks a stream incomplete when it happens before the protocol's
// terminal event. A terminal receipt that was already observed (or is still
// buffered without its blank-line separator) closes the provider's work, so it
// must settle exactly instead of retaining the reservation as an estimate.
func TestResponseUsageInterruptionAfterTerminalReceipt(t *testing.T) {
	const delta = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial output text\"}\n\n"
	const completed = "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":19,\"total_tokens\":92}}}"
	for _, tc := range []struct {
		name, body  string
		incomplete  bool
		input, outp int
	}{
		{"responses completed then reset", delta + completed + "\n\n", false, 73, 19},
		{"responses completed buffered without separator", delta + completed, false, 73, 19},
		{"responses failed with usage then reset", delta + "data: {\"type\":\"response.failed\",\"response\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":19}}}\n\n", false, 73, 19},
		{"responses incomplete with usage then reset", "data: {\"type\":\"response.incomplete\",\"response\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":19}}}\n\n", false, 73, 19},
		{"chat usage then DONE then reset", "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\ndata: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19}}\n\ndata: [DONE]\n\n", false, 73, 19},
		{"claude message_stop then reset", "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":1}}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":19}}\n\ndata: {\"type\":\"message_stop\"}\n\n", false, 73, 19},
		// Controls: interruptions before the terminal event stay incomplete.
		{"deltas then reset", delta, true, 0, 0},
		{"provisional usage then reset", "data: {\"type\":\"response.in_progress\",\"response\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":0}}}\n\n" + delta, true, 0, 0},
		{"chat usage without DONE then reset", "data: {\"usage\":{\"prompt_tokens\":73,\"completion_tokens\":19}}\n\n", true, 73, 0},
		{"claude delta without message_stop then reset", "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":73,\"output_tokens\":1}}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":19}}\n\n", true, 73, 0},
		{"truncated terminal JSON then reset", delta + completed[:len(completed)-20], true, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewResponseUsageAccumulator(true)
			a.Observe([]byte(tc.body))
			a.MarkIncomplete()
			usage := a.Finish(0, nil)
			if tc.incomplete {
				require.Equal(t, "response_usage_incomplete_stream", usage.BillingEstimateReason)
				require.GreaterOrEqual(t, usage.PromptTokens, tc.input)
				return
			}
			require.Empty(t, usage.BillingEstimateReason, "a closed receipt must settle exactly")
			require.Equal(t, tc.input, usage.PromptTokens)
			require.Equal(t, tc.outp, usage.CompletionTokens)
		})
	}
}
