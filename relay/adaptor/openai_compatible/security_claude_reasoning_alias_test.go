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

// TestSecurityClaudeReasoningAliases ensures shared normalization does not lose
// a converter-supported reasoning alias or double-count equivalent fields.
func TestSecurityClaudeReasoningAliases(t *testing.T) {
	const reasoning = "careful explanation of the answer"
	for _, keys := range [][]string{{"reasoning_content"}, {"reasoning"}, {"thinking"}, {"reasoning_content", "reasoning", "thinking"}} {
		t.Run(strings.Join(keys, "+"), func(t *testing.T) {
			delta := map[string]any{}
			for _, key := range keys {
				delta[key] = reasoning
			}
			event, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta}}})
			require.NoError(t, err)
			wire := "data: " + string(event) + "\n\n" + "data: [DONE]\n\n"
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			usage, apiErr := ConvertOpenAIStreamToClaudeSSE(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(wire))}, 10, "gpt-4")
			require.Nil(t, apiErr)
			require.Equal(t, 10, usage.PromptTokens)
			require.Equal(t, CountTokenText(reasoning, "gpt-4"), usage.CompletionTokens)
			require.NotEmpty(t, usage.BillingEstimateReason)
		})
	}
}
