package vertexai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestVertexNanoClaudeImageConversion checks the actual Vertex response adapter boundary.
func TestVertexNanoClaudeImageConversion(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"hello"},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1121,"totalTokenCount":1122}}`))}
	usage, failure := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{ActualModelName: "gemini-nano-banana-2.1"})
	require.Nil(t, failure)
	require.Nil(t, usage, "controller must receive the converted Claude response")
	raw, ok := c.Get(ctxkey.ConvertedResponse)
	require.True(t, ok, "Claude conversion was bypassed: %s", rec.Body.String())
	converted := raw.(*http.Response)
	body, err := io.ReadAll(converted.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"type":"message"`)
	require.Contains(t, string(body), "data:image/png;base64,aW1hZ2U=")
}

// TestVertexNanoClaudeImageStream checks the streaming response is a Claude event stream.
func TestVertexNanoClaudeImageStream(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	payload := `{"candidates":[{"content":{"parts":[{"text":"hello"},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}]}`
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\ndata: [DONE]\n\n"))}
	usage, failure := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{ActualModelName: "gemini-nano-banana-2.1", IsStream: true})
	require.Nil(t, failure)
	require.Nil(t, usage)
	raw, ok := c.Get(ctxkey.ConvertedResponse)
	require.True(t, ok)
	body, err := io.ReadAll(raw.(*http.Response).Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "event: content_block_delta")
	require.Contains(t, string(body), "data:image/png;base64,aW1hZ2U=")
}
