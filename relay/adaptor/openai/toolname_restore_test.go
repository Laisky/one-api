package openai

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
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestOpenAIHandler_RestoresSanitizedToolName drives the openai package's
// non-stream Handler with a fake upstream response that mentions the
// sanitized name and asserts the client sees the original.
func TestOpenAIHandler_RestoresSanitizedToolName(t *testing.T) {
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

	herr, usage := Handler(c, upstream, 0, "gpt-4o-mini")
	require.Nil(t, herr)
	require.NotNil(t, usage)

	clientBody := w.Body.String()
	require.Contains(t, clientBody, `"name":"server.tool_name"`)
	require.NotContains(t, clientBody, `"name":"server_tool_name"`)
}

// TestOpenAIStreamHandler_RestoresSanitizedToolName feeds the streaming
// handler an SSE chunk that carries the sanitized name and asserts the
// client-facing stream re-emits the original identifier.
func TestOpenAIStreamHandler_RestoresSanitizedToolName(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{
		"server_tool_name": "server.tool_name",
	})

	idx := 0
	chunk := openai_compatible.ChatCompletionsStreamResponse{
		Id:      "chatcmpl-test",
		Object:  "chat.completion.chunk",
		Created: 1700000000,
		Model:   "gpt-4o-mini",
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
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

	errResp, _, _ := StreamHandler(c, resp, relaymode.ChatCompletions)
	require.Nil(t, errResp)

	body := w.Body.String()
	require.Contains(t, body, `"name":"server.tool_name"`)
	require.NotContains(t, body, `"name":"server_tool_name"`)
}
