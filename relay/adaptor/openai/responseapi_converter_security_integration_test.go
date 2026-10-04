package openai

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurityResponseConverterRejectsOversizedInput exercises the production
// converter entry point, including when conversation persistence is disabled.
func TestSecurityResponseConverterRejectsOversizedInput(t *testing.T) {
	request := &ResponseAPIRequest{Input: make([]any, 4097)}
	for i := range request.Input {
		request.Input[i] = "x"
	}
	if _, err := ConvertResponseAPIToChatCompletionRequest(request); err == nil {
		require.FailNow(t, "converter accepted oversized effective input")
	}
}

// TestSecurityResponseConverterFIFOAndTurnIsolation verifies allocator lifetime:
// reasoning does not break a turn, output IDs pair FIFO, and a new turn can reuse
// the original ID rather than inheriting another turn's suffix allocation state.
func TestSecurityResponseConverterFIFOAndTurnIsolation(t *testing.T) {
	call := func(name string) map[string]any {
		return map[string]any{"type": "function_call", "call_id": "call_repeated", "name": name, "arguments": "{}"}
	}
	output := func(value string) map[string]any {
		return map[string]any{"type": "function_call_output", "call_id": "call_repeated", "output": value}
	}
	request := &ResponseAPIRequest{Input: []any{
		map[string]any{"role": "assistant", "content": "first turn"},
		call("one"), map[string]any{"type": "reasoning", "content": "reasoning state"}, call("two"),
		output("result one"), output("result two"), "next user turn", call("three"), output("result three"),
	}}
	converted, err := ConvertResponseAPIToChatCompletionRequest(request)
	if err != nil {
		require.NoError(t, err)
	}
	if len(converted.Messages) != 6 {
		require.FailNowf(t, "security regression", "expected six messages, got %d", len(converted.Messages))
	}
	first := converted.Messages[0]
	if len(first.ToolCalls) != 2 || first.ToolCalls[0].Id == first.ToolCalls[1].Id {
		require.FailNow(t, "duplicate calls were not kept in one collision-free turn")
	}
	if converted.Messages[1].ToolCallId != first.ToolCalls[0].Id || converted.Messages[2].ToolCallId != first.ToolCalls[1].Id {
		require.FailNow(t, "duplicate call outputs were not paired FIFO")
	}
	if first.ReasoningContent == nil || *first.ReasoningContent != "reasoning state" {
		require.FailNow(t, "reasoning no longer belongs to its assistant turn")
	}
	last := converted.Messages[4]
	if len(last.ToolCalls) != 1 || last.ToolCalls[0].Id != first.ToolCalls[0].Id || converted.Messages[5].ToolCallId != last.ToolCalls[0].Id {
		require.FailNow(t, "allocator leaked state across assistant turns")
	}
}

// BenchmarkSecurityResponseConverterDuplicateIDs measures the real converter's
// bounded duplicate-ID workload before and after its allocator is replaced.
func BenchmarkSecurityResponseConverterDuplicateIDs(b *testing.B) {
	for _, n := range []int{256, 512, 1024} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			request := &ResponseAPIRequest{Input: make([]any, n)}
			for i := range request.Input {
				request.Input[i] = map[string]any{"type": "function_call", "call_id": "duplicate", "name": "fixture", "arguments": "{}"}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ConvertResponseAPIToChatCompletionRequest(request)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
