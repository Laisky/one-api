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

// TestSecurityOriginalReasoningUsage preserves every billable component independently of presentation options.
func TestSecurityOriginalReasoningUsage(t *testing.T) {
	reasoning := strings.Repeat("解释 reasoning step. ", 1000)
	toolArgs := `{"answer":"你好"}`
	cases := []struct {
		name    string
		message map[string]any
		pieces  []string
	}{
		{"native", map[string]any{"content": "answer", "reasoning_content": reasoning}, []string{"answer", reasoning}},
		{"reasoning-alias", map[string]any{"content": "answer", "reasoning": reasoning}, []string{"answer", reasoning}},
		{"thinking-alias", map[string]any{"content": "answer", "thinking": reasoning}, []string{"answer", reasoning}},
		{"duplicate-aliases", map[string]any{"content": "answer", "reasoning_content": reasoning, "reasoning": reasoning, "thinking": reasoning}, []string{"answer", reasoning}},
		{"extracted", map[string]any{"content": "<think>" + reasoning + "</think>answer"}, []string{"<think>" + reasoning + "</think>answer"}},
		{"mixed-distinct", map[string]any{"content": "<think>inside</think>answer", "reasoning_content": reasoning}, []string{"<think>inside</think>answer", reasoning}},
		{"mixed-duplicate", map[string]any{"content": "<think>" + reasoning + "</think>answer", "reasoning": reasoning}, []string{"<think>" + reasoning + "</think>answer"}},
		{"reasoning-only", map[string]any{"content": "", "reasoning_content": reasoning}, []string{reasoning}},
		{"tools", map[string]any{"content": "answer", "thinking": reasoning, "tool_calls": []any{map[string]any{"id": "call1", "type": "function", "function": map[string]any{"name": "f", "arguments": toolArgs}}}}, []string{"answer", reasoning, toolArgs}},
	}
	for _, tc := range cases {
		for _, extract := range []bool{false, true} {
			for _, specialized := range []bool{false, true} {
				for _, measured := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/extract=%v/specialized=%v/measured=%v", tc.name, extract, specialized, measured), func(t *testing.T) {
						expected := 0
						for _, piece := range tc.pieces {
							expected += CountTokenText(piece, "gpt-4")
						}
						upstreamUsage := map[string]any{"prompt_tokens": 10}
						if measured {
							upstreamUsage["completion_tokens"] = 7
							upstreamUsage["total_tokens"] = 17
							expected = 7
						}
						payload, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "message": tc.message, "finish_reason": "stop"}}, "usage": upstreamUsage})
						require.NoError(t, err)
						c, _ := gin.CreateTestContext(httptest.NewRecorder())
						c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/chat/completions?thinking=%v&reasoning_format=thinking", extract), nil)
						response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(payload)))}
						handler := Handler
						if specialized {
							handler = HandlerWithThinking
						}
						apiErr, usage := handler(c, response, 10, "gpt-4")
						require.Nil(t, apiErr)
						require.NotNil(t, usage)
						require.Equal(t, expected, usage.CompletionTokens)
						require.Equal(t, 10+expected, usage.TotalTokens)
						if measured {
							require.Empty(t, usage.BillingEstimateReason)
						} else {
							require.NotEmpty(t, usage.BillingEstimateReason)
						}
					})
				}
			}
		}
	}
}
