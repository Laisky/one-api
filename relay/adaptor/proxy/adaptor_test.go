package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// TestDoResponseForwardsBodyAndReturnsOpenAIUsage verifies proxied OpenAI responses remain unchanged while usage is billed.
func TestDoResponseForwardsBodyAndReturnsOpenAIUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"id":"chatcmpl_1","choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":42,"completion_tokens":83,"total_tokens":125}}`
	ctx, recorder := proxyTestContext()
	resp := proxyHTTPResponse(body, "application/json")

	usage, err := (&Adaptor{}).DoResponse(ctx, resp, &meta.Meta{PromptTokens: 42, ActualModelName: "gpt-4o-mini"})

	require.Nil(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, body, recorder.Body.String())
	require.NotNil(t, usage)
	require.Equal(t, 42, usage.PromptTokens)
	require.Equal(t, 83, usage.CompletionTokens)
	require.Equal(t, 125, usage.TotalTokens)
}

// TestProxyUsageFromResponseSupportsResponseAPIAndClaudeUsage verifies proxy billing handles all supported API usage formats.
func TestProxyUsageFromResponseSupportsResponseAPIAndClaudeUsage(t *testing.T) {
	responseAPIUsage := proxyUsageFromResponse([]byte(`{"usage":{"input_tokens":7,"output_tokens":11,"total_tokens":18}}`), nil)
	require.NotNil(t, responseAPIUsage)
	require.Equal(t, 7, responseAPIUsage.PromptTokens)
	require.Equal(t, 11, responseAPIUsage.CompletionTokens)
	require.Equal(t, 18, responseAPIUsage.TotalTokens)

	claudeUsage := proxyUsageFromResponse([]byte(`{"usage":{"input_tokens":13,"output_tokens":17}}`), nil)
	require.NotNil(t, claudeUsage)
	require.Equal(t, 13, claudeUsage.PromptTokens)
	require.Equal(t, 17, claudeUsage.CompletionTokens)
	require.Equal(t, 30, claudeUsage.TotalTokens)
}

// TestProxyUsageFromResponseParsesStreamingUsage verifies stream passthrough billing uses upstream usage chunks.
func TestProxyUsageFromResponseParsesStreamingUsage(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	usage := proxyUsageFromResponse([]byte(stream), &meta.Meta{PromptTokens: 5, ActualModelName: "gpt-4o-mini"})

	require.NotNil(t, usage)
	require.Equal(t, 5, usage.PromptTokens)
	require.Equal(t, 8, usage.CompletionTokens)
	require.Equal(t, 13, usage.TotalTokens)
}

// TestProxyUsageFromResponseFallsBackToPromptAndCompletionEstimate verifies missing upstream usage does not become zero billing.
func TestProxyUsageFromResponseFallsBackToPromptAndCompletionEstimate(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"hello world"}}]}`)

	usage := proxyUsageFromResponse(body, &meta.Meta{PromptTokens: 9, ActualModelName: "gpt-4o-mini"})

	require.NotNil(t, usage)
	require.Equal(t, 9, usage.PromptTokens)
	require.Greater(t, usage.TotalTokens, 9)
}

// proxyTestContext creates a Gin test context and response recorder.
func proxyTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return ctx, recorder
}

// proxyHTTPResponse creates an upstream HTTP response with the provided body and content type.
func proxyHTTPResponse(body string, contentType string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{contentType},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}
