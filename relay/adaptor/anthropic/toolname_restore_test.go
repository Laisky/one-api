package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
)

// TestAnthropicHandler_RestoresSanitizedToolName feeds a synthetic Anthropic
// non-streaming response carrying a `tool_use` block whose `name` matches the
// sanitized identifier stashed on the gin context. After conversion to the
// OpenAI shape, the client-facing payload must surface the original name.
func TestAnthropicHandler_RestoresSanitizedToolName(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-restore-test"))

	c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{
		"server_tool_name": "server.tool_name",
	})

	stopReason := "tool_use"
	body := Response{
		Id:    "msg_test",
		Type:  "message",
		Role:  "assistant",
		Model: "claude-sonnet-4-5",
		Content: []Content{
			{
				Type:  "tool_use",
				Id:    "tool_call_1",
				Name:  "server_tool_name",
				Input: map[string]any{"q": "hi"},
			},
		},
		StopReason: &stopReason,
		Usage:      Usage{InputTokens: 5, OutputTokens: 7},
	}
	bodyJSON, err := json.Marshal(body)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(bodyJSON))),
	}

	errResp, usage := Handler(c, resp, 0, "claude-sonnet-4-5")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	out := recorder.Body.String()
	require.Contains(t, out, `"name":"server.tool_name"`, "client should see original name")
	require.NotContains(t, out, `"name":"server_tool_name"`, "sanitized name must not leak to client")
}

// TestAnthropicStreamHandler_RestoresSanitizedToolName drives StreamHandler
// with an SSE stream carrying a `content_block_start` event for a `tool_use`
// block bearing the sanitized name, and asserts the converted OpenAI-style
// chunks carry the original identifier.
func TestAnthropicStreamHandler_RestoresSanitizedToolName(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-restore-stream-test"))

	c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{
		"server_tool_name": "server.tool_name",
	})

	sse := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_a","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[],"usage":{"input_tokens":3,"output_tokens":0}}}`,
		``,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_call_1","name":"server_tool_name","input":{}}}`,
		``,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"hi\"}"}}`,
		``,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":3,"output_tokens":7}}`,
		``,
	}, "\n")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	body := recorder.Body.String()
	require.Contains(t, body, `"name":"server.tool_name"`, "client should see original name in stream")
	require.NotContains(t, body, `"name":"server_tool_name"`, "sanitized name must not leak to client")
}
