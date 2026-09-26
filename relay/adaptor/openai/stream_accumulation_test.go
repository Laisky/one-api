package openai

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/relaymode"
)

// accumulationFixture constructs independent expected text and wire events for normal and oversized streams.
func accumulationFixture(t testing.TB, mode, count, size int) (string, string, string) {
	t.Helper()
	var wire, content, reasoning strings.Builder
	for i := 0; i < count; i++ {
		piece := fmt.Sprintf("%d:世界🙂:", i) + strings.Repeat("x", size)
		thought := fmt.Sprintf("thinking-%d;", i)
		choice := map[string]any{"index": 0, "finish_reason": nil}
		if mode == relaymode.ChatCompletions {
			choice["delta"] = map[string]string{"content": piece, "reasoning_content": thought}
			reasoning.WriteString(thought)
		} else {
			choice["text"] = piece
		}
		payload, err := json.Marshal(map[string]any{"id": "accumulation", "choices": []any{choice}})
		require.NoError(t, err)
		wire.WriteString("data: ")
		wire.Write(payload)
		wire.WriteString("\n\n")
		content.WriteString(piece)
	}
	wire.WriteString("data: [DONE]\n\n")
	return wire.String(), reasoning.String() + content.String(), reasoning.String()
}

// TestStreamHandlerAccumulationPreservesOutput checks both accumulation paths, Unicode ordering and converted trace values.
func TestStreamHandlerAccumulationPreservesOutput(t *testing.T) {
	for _, mode := range []int{relaymode.ChatCompletions, relaymode.Completions} {
		for _, size := range []int{128, 70 * 1024} {
			t.Run(fmt.Sprintf("mode-%d/size-%d", mode, size), func(t *testing.T) {
				count := 1024
				if size > 1024 {
					count = 4
				}
				wire, expected, reasoning := accumulationFixture(t, mode, count, size)
				c, writer := newTestGinContext()
				err, text, usage := StreamHandler(c, newSSEResponse(wire), mode)
				require.Nil(t, err)
				require.Nil(t, usage)
				require.Equal(t, expected, text)
				require.Equal(t, 1, strings.Count(writer.Body.String(), "data: [DONE]"))
				converted, exists := c.Get(ctxkey.ConvertedResponse)
				require.True(t, exists)
				values, ok := converted.(map[string]any)
				require.True(t, ok)
				require.Equal(t, expected, values["content"])
				require.Equal(t, reasoning, values["reasoning"])
			})
		}
	}
}

// BenchmarkStreamHandlerLongResponse isolates accumulated text allocations; E2E results remain the acceptance evidence.
func BenchmarkStreamHandlerLongResponse(b *testing.B) {
	wire, expected, _ := accumulationFixture(b, relaymode.ChatCompletions, 1024, 128)
	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()
	for b.Loop() {
		c, _ := newTestGinContext()
		err, text, _ := StreamHandler(c, newSSEResponse(wire), relaymode.ChatCompletions)
		require.Nil(b, err)
		require.Equal(b, expected, text)
	}
}
