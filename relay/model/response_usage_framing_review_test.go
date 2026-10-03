package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseUsageFramingAndProvisionalInput exercises complete SSE envelopes,
// not individual JSON-looking data lines. Responses lifecycle placeholders must
// not suppress input estimation when the final receipt omits that counter.
func TestResponseUsageFramingAndProvisionalInput(t *testing.T) {
	cases := []struct {
		name, body    string
		input, output int
		estimated     bool
	}{
		{
			name: "multiline object value is not an independent event",
			body: "data: {\"usage\":\ndata: {\"prompt_tokens\":7,\"completion_tokens\":11}\ndata: }\n\n",
			input: 7, output: 11,
		},
		{
			name: "multiline nested receipt after lifecycle placeholder",
			body: "data: {\"type\":\"response.created\",\"response\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":\ndata: {\"usage\":{\"input_tokens\":7,\"output_tokens\":11}}\ndata: }\n\n",
			input: 7, output: 11,
		},
		{
			name: "provisional Responses zero does not erase input estimate",
			body: "data: {\"type\":\"response.created\",\"response\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"output_tokens\":11}}}\n\n",
			input: 29, output: 11, estimated: true,
		},
		{
			name: "final Responses zero remains authoritative",
			body: "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":11}}}\n\n",
			input: 0, output: 11,
		},
		{
			name: "Claude message start input is measured not provisional",
			body: "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n" +
				"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":11}}\n\n",
			input: 0, output: 11,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, newline := range []string{"\n", "\r\n", "\r"} {
				body := strings.ReplaceAll(tc.body, "\n", newline)
				for _, chunk := range []int{1, 7, len(body)} {
					t.Run(fmt.Sprintf("newline-%q/chunk-%d", newline, chunk), func(t *testing.T) {
						acc := NewResponseUsageAccumulator(true)
						for offset := 0; offset < len(body); offset += chunk {
							acc.Observe([]byte(body[offset:min(offset+chunk, len(body))]))
						}
						usage := acc.Finish(29, nil)
						require.Equal(t, tc.input, usage.PromptTokens)
						require.Equal(t, tc.output, usage.CompletionTokens)
						require.Equal(t, tc.input+tc.output, usage.TotalTokens)
						require.Equal(t, tc.estimated, usage.BillingEstimateReason != "")
						require.Equal(t, usage, acc.Finish(29, nil), "finalization must be idempotent")
					})
				}
			}
		})
	}
}
