package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestSecurityClaudeJSONFallbackUsage exercises delivered Claude output against raw provider fallback accounting.
func TestSecurityClaudeJSONFallbackUsage(t *testing.T) {
	text := strings.Repeat("synthetic accounting evidence ", 100)
	payload := `{"answer":"` + text + `"}`
	quoted, err := json.Marshal(payload)
	require.NoError(t, err)
	delta := `{"type":"response.output_json.delta","output_index":0,"delta":{"partial_json":` + string(quoted) + `}}`
	done := `{"type":"response.output_json.done","output_index":0,"json":` + payload + `}`
	object := `{"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_local","function":{"name":"lookup","arguments":` + payload + `}}]}}]}`
	cases := []struct {
		name, wire string
		measured   bool
	}{
		{"json_delta_done", "data: " + delta + "\n\n" + "data: " + done + "\n\n", false},
		{"json_done_only", "data: " + done + "\n\n", false},
		{"chat_object_arguments", "data: " + object + "\n\n", false},
		{"measured_control", "data: " + object + "\n\n" + `data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":3}}` + "\n\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			usage, apiErr := ConvertOpenAIStreamToClaudeSSE(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.wire + "data: [DONE]\n\n"))}, 11, "gpt-4")
			require.Nil(t, apiErr)
			require.Contains(t, w.Body.String(), text, "converter actually delivered the structured content")
			if tc.measured {
				require.Equal(t, 3, usage.CompletionTokens)
				require.Empty(t, usage.BillingEstimateReason)
			} else {
				require.GreaterOrEqual(t, usage.CompletionTokens, 200, "100 ordinary repeated phrases contain at least 200 tokens independently of the accumulator")
				require.Less(t, usage.CompletionTokens, 900, "a matching done snapshot must not double the JSON estimate")
				require.NotEmpty(t, usage.BillingEstimateReason)
			}
		})
	}
}
