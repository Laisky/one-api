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

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// TestHandler_RestoresSanitizedToolName feeds the non-streaming handler an
// upstream payload whose `tool_calls[].function.name` matches a sanitized
// identifier stashed on the gin context, and asserts the client-facing JSON
// carries the original (un-sanitized) name.
func TestHandler_RestoresSanitizedToolName(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{
		"server_tool_name": "server.tool_name",
	})

	respStruct := SlimTextResponse{
		Choices: []TextResponseChoice{
			{
				Index: 0,
				Message: model.Message{
					Role: "assistant",
					ToolCalls: []model.Tool{
						{
							Id:   "call_1",
							Type: "function",
							Function: &model.Function{
								Name:      "server_tool_name",
								Arguments: "{}",
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: model.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}
	body, err := json.Marshal(respStruct)
	require.NoError(t, err)

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}

	herr, usage := Handler(c, upstream, 0, "deepseek-chat")
	require.Nil(t, herr)
	require.NotNil(t, usage)

	// Client-facing body must carry the original name.
	clientBody := w.Body.String()
	require.Contains(t, clientBody, `"name":"server.tool_name"`)
	require.NotContains(t, clientBody, `"name":"server_tool_name"`)
}

// TestUnifiedStreamProcessing_RestoresSanitizedToolName verifies the streaming
// pipeline rewrites sanitized tool names back to originals as delta chunks are
// forwarded to the client.
func TestUnifiedStreamProcessing_RestoresSanitizedToolName(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{
		"server_tool_name": "server.tool_name",
	})

	idx := 0
	chunk := ChatCompletionsStreamResponse{
		Id:      "chatcmpl-test",
		Object:  "chat.completion.chunk",
		Created: 1700000000,
		Model:   "deepseek-chat",
		Choices: []ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: model.Message{
					Role: "assistant",
					ToolCalls: []model.Tool{
						{
							Id:    "call_1",
							Type:  "function",
							Index: &idx,
							Function: &model.Function{
								Name:      "server_tool_name",
								Arguments: "{}",
							},
						},
					},
				},
			},
		},
	}
	chunkJSON, err := json.Marshal(chunk)
	require.NoError(t, err)

	sse := "data: " + string(chunkJSON) + "\n\ndata: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}

	errResp, _ := UnifiedStreamProcessing(c, resp, 0, "deepseek-chat", false)
	require.Nil(t, errResp)

	body := w.Body.String()
	require.Contains(t, body, `"name":"server.tool_name"`, "client should see original name in stream")
	require.NotContains(t, body, `"name":"server_tool_name"`, "sanitized name must not leak to client")
}
