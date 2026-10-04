package openai_compatible

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestCompletionFragmentEstimate preserves sub-token fragments across choices
// and distinct billable fields while deduplicating native reasoning aliases.
func TestCompletionFragmentEstimate(t *testing.T) {
	cases := []struct {
		name     string
		messages []map[string]any
		want     int
	}{
		{"four-choices", []map[string]any{{"content": "abc"}, {"content": "abc"}, {"content": "abc"}, {"content": "abc"}}, 3},
		{"same-bytes-distinct-fields", []map[string]any{{"content": "abc", "reasoning_content": "abc", "tool_calls": []any{map[string]any{"id": "call1", "type": "function", "function": map[string]any{"name": "f", "arguments": "abc"}}}}}, 2},
		{"reasoning-aliases-once", []map[string]any{{"content": "xyz", "reasoning_content": "abc", "reasoning": "abc", "thinking": "abc"}}, 1},
		{"same-reasoning-distinct-choices", []map[string]any{{"reasoning_content": "abc"}, {"reasoning_content": "abc"}, {"reasoning_content": "abc"}, {"reasoning_content": "abc"}}, 3},
		{"tool-calls-distinct", []map[string]any{{"content": "abc", "tool_calls": []any{map[string]any{"id": "call1", "type": "function", "function": map[string]any{"name": "f", "arguments": "abc"}}, map[string]any{"id": "call2", "type": "function", "function": map[string]any{"name": "f", "arguments": "abc"}}}}}, 2},
		{"extracted-alias-once", []map[string]any{{"content": "<think>abc</think>x", "reasoning": "abc", "thinking": "abc"}}, 4},
	}
	for _, tc := range cases {
		for _, specialized := range []bool{false, true} {
			for _, thinking := range []bool{false, true} {
				for _, measured := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/specialized=%v/thinking=%v/measured=%v", tc.name, specialized, thinking, measured), func(t *testing.T) {
						choices := make([]any, 0, len(tc.messages))
						for index, message := range tc.messages {
							choices = append(choices, map[string]any{"index": index, "message": message, "finish_reason": "stop"})
						}
						receipt := map[string]any{"prompt_tokens": 10}
						want := tc.want
						if measured {
							receipt["completion_tokens"] = 7
							receipt["total_tokens"] = 17
							want = 7
						}
						payload, err := json.Marshal(map[string]any{"choices": choices, "usage": receipt})
						require.NoError(t, err)
						c, _ := gin.CreateTestContext(httptest.NewRecorder())
						c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/chat/completions?thinking=%v&reasoning_format=thinking", thinking), nil)
						response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(payload)))}
						handler := Handler
						if specialized {
							handler = HandlerWithThinking
						}
						apiErr, usage := handler(c, response, 10, "deepseek-chat")
						require.Nil(t, apiErr)
						require.NotNil(t, usage)
						require.Equal(t, want, usage.CompletionTokens, "round only after collecting all original billable fragments")
						require.Equal(t, 10+want, usage.TotalTokens)
						require.Equal(t, !measured, usage.BillingEstimateReason != "")
					})
				}
			}
		}
	}
}
