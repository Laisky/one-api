package vertexai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestDoResponseStreamInheritsGeminiUsageMetadata verifies the Vertex AI Gemini adapter
// delegates streaming to gemini.StreamHandler and prefers the authoritative upstream
// usageMetadata (cached prompt tokens + reasoning) over the text-based estimate.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestDoResponseStreamInheritsGeminiUsageMetadata(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	chunk := `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}],` +
		`"usageMetadata":{"promptTokenCount":50000,"cachedContentTokenCount":48000,"candidatesTokenCount":100,"thoughtsTokenCount":200,"totalTokenCount":50300}}`
	sse := "data: " + chunk + "\n\n" + "data: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}

	a := &Adaptor{}
	usage, apiErr := a.DoResponse(c, resp, &meta.Meta{
		IsStream:        true,
		Mode:            relaymode.ChatCompletions,
		ActualModelName: "gemini-2.5-flash",
		PromptTokens:    123,
	})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 50000, usage.PromptTokens)
	require.Equal(t, 300, usage.CompletionTokens)
	require.Equal(t, 50300, usage.TotalTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 48000, usage.PromptTokensDetails.CachedTokens)
}
