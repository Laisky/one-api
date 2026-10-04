package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestSecurityClaudeStreamUsageNormalization compares the actual converter with the shared evidence normalizer.
func TestSecurityClaudeStreamUsageNormalization(t *testing.T) {
	cases := []struct {
		name, wire         string
		prompt, completion int
		estimated          bool
	}{
		{"total-only", `{"total_tokens":100}`, 10, 90, true},
		{"total-below-prompt", `{"total_tokens":3}`, 10, 0, true},
		{"measured", `{"prompt_tokens":7,"completion_tokens":8,"total_tokens":15}`, 7, 8, false},
		{"measured-zero", `{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`, 0, 0, false},
		{"invalid-negative", `{"prompt_tokens":-1,"completion_tokens":-2}`, 10, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire := "data: {\"choices\":[],\"usage\":" + tc.wire + "}\n\n"
			wire += wire + "data: [DONE]\n\n"
			c, w := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			usage, apiErr := ConvertOpenAIStreamToClaudeSSE(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(wire))}, 10, "gpt-4")
			require.Nil(t, apiErr)
			require.Equal(t, tc.prompt, usage.PromptTokens)
			require.Equal(t, tc.completion, usage.CompletionTokens)
			require.Equal(t, tc.prompt+tc.completion, usage.TotalTokens)
			require.Equal(t, tc.estimated, usage.BillingEstimateReason != "")
			observer := relaymodel.NewResponseUsageAccumulator(true)
			observer.Observe([]byte(wire))
			require.Equal(t, observer.Finish(10, func(text string) int { return CountTokenText(text, "gpt-4") }), usage)
			_ = w
		})
	}
}

// TestSecurityClaudeStreamPartialReceipt keeps earlier measured input and cache dimensions across later output-only updates.
func TestSecurityClaudeStreamPartialReceipt(t *testing.T) {
	wire := "data: {\"usage\":{\"prompt_tokens\":7,\"prompt_tokens_details\":{\"cached_tokens\":3}}}\n\n" +
		"data: {\"usage\":{\"completion_tokens\":8}}\n\n" + "data: [DONE]\n\n"
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	usage, apiErr := ConvertOpenAIStreamToClaudeSSE(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(wire))}, 10, "gpt-4")
	require.Nil(t, apiErr)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 8, usage.CompletionTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 3, usage.PromptTokensDetails.CachedTokens)
	var last map[string]any
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		if event["type"] == "message_delta" {
			if val, ok := event["usage"].(map[string]any); ok {
				last = val
			}
		}
	}
	require.EqualValues(t, 7, last["input_tokens"])
	require.EqualValues(t, 8, last["output_tokens"])
}
