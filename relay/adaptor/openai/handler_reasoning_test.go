package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

// TestHandlerRemapsReasoningFormatThinking verifies reasoning_content converts to thinking when requested.
func TestHandlerRemapsReasoningFormatThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=true&reasoning_format=thinking", nil)
	c.Request = req

	reasoning := "deep dive"
	respStruct := SlimTextResponse{
		Choices: []TextResponseChoice{
			{
				Index: 0,
				Message: model.Message{
					Role:             "assistant",
					Content:          "2",
					ReasoningContent: &reasoning,
				},
				FinishReason: "stop",
			},
		},
		Usage: model.Usage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8},
	}

	body, err := json.Marshal(respStruct)
	require.NoError(t, err, "failed to marshal upstream response")

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	errResp, _ := Handler(c, upstream, 0, "gpt-4o")
	require.Nil(t, errResp, "handler returned unexpected error")
	require.Equal(t, http.StatusOK, w.Code, "unexpected status code")

	var out SlimTextResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), "failed to unmarshal handler output")
	require.Len(t, out.Choices, 1, "expected one choice")

	msg := out.Choices[0].Message
	require.NotNil(t, msg.Thinking, "expected thinking to be set")
	require.Equal(t, "deep dive", *msg.Thinking, "expected thinking to contain reasoning text")
	require.Nil(t, msg.ReasoningContent, "expected reasoning_content cleared")
}
