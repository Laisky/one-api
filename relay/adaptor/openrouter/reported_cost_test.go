package openrouter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// newReportedCostContext builds a gin context for one OpenRouter response.
// Parameters: t owns the recorder. Returns: the context and its recorder.
func newReportedCostContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, w
}

// reportedCostResponse wraps body as a provider response. Parameters: body is
// the payload and stream selects SSE. Returns: the response.
func reportedCostResponse(body string, stream bool) *http.Response {
	header := http.Header{"Content-Type": []string{"application/json"}}
	if stream {
		header.Set("Content-Type", "text/event-stream")
	}
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

// TestDoResponseAttachesReportedCost proves DoResponse carries OpenRouter's
// usage.cost (plus the provider-billed upstream cost for BYOK requests) to the
// usage settlement prices, for JSON, SSE and converted Claude responses, and
// never echoes it to the client.
func TestDoResponseAttachesReportedCost(t *testing.T) {
	const chatUsage = `"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"cost":0.25,"is_byok":false,"cost_details":{"upstream_inference_cost":null}}`
	for _, tc := range []struct {
		name   string
		body   string
		stream bool
		claude bool
		want   float64
	}{
		{name: "json", body: `{"id":"gen","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],` + chatUsage + `}`, want: 0.25},
		{name: "json_byok", body: `{"id":"gen","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"cost":0.01,"is_byok":true,"cost_details":{"upstream_inference_cost":0.5}}}`, want: 0.51},
		{name: "sse", stream: true, body: "data: {\"id\":\"gen\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"id\":\"gen\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]," + chatUsage + "}\n\ndata: [DONE]\n\n", want: 0.25},
		{name: "claude_json", claude: true, body: `{"id":"gen","object":"chat.completion","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],` + chatUsage + `}`, want: 0.25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newReportedCostContext(t)
			info := &meta.Meta{IsStream: tc.stream, ActualModelName: "m", PromptTokens: 10}
			c.Set(ctxkey.Meta, info)
			if tc.claude {
				c.Set(ctxkey.ClaudeMessagesConversion, true)
			}
			usage, apiErr := (&Adaptor{}).DoResponse(c, reportedCostResponse(tc.body, tc.stream), info)
			require.Nil(t, apiErr)
			if tc.claude {
				stored, ok := c.Get(ctxkey.ConvertedResponseUsage)
				require.True(t, ok)
				usage, _ = stored.(*model.Usage)
			}
			require.NotNil(t, usage)
			require.InDelta(t, tc.want, usage.UpstreamCostUSD, 1e-12)
			require.NotContains(t, w.Body.String(), `"cost"`, "the provider cost must not reach the client")
		})
	}
}

// TestScrubReportedCostLine proves streamed usage chunks lose only the
// operator-side cost fields and every other line passes through unchanged.
func TestScrubReportedCostLine(t *testing.T) {
	scrubbed := ScrubReportedCostLine(`data: {"id":"gen","choices":[],"usage":{"prompt_tokens":3,"cost":1.5,"is_byok":false,"cost_details":{"upstream_inference_cost":null}}}`)
	require.NotContains(t, scrubbed, "cost")
	require.NotContains(t, scrubbed, "is_byok")
	require.Contains(t, scrubbed, `"prompt_tokens":3`)
	for _, line := range []string{
		`data: {"id":"gen","choices":[{"delta":{"content":"the cost is 3"}}]}`,
		`data: [DONE]`,
		`data: {not json "cost"}`,
		`data: {"id":"gen","usage":{"prompt_tokens":3}}`,
	} {
		require.Equal(t, line, ScrubReportedCostLine(line))
	}
}
