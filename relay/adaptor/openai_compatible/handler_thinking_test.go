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

	"github.com/songquanpeng/one-api/relay/model"
)

// TestHandler_NonStream_ThinkingParam verifies non-stream handler respects thinking and reasoning_format
func TestHandler_NonStream_ThinkingParam(t *testing.T) {
	t.Parallel()
	// Build a simple non-stream response with a single choice containing <think>
	respStruct := SlimTextResponse{
		Choices: []TextResponseChoice{
			{
				Index:        0,
				Message:      structToMessage("before <think>xyz</think> after"),
				FinishReason: "stop",
			},
		},
		Usage: modelUsage(0, 0),
	}
	b, _ := json.Marshal(respStruct)

	// Prepare gin context with query params
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/chat/completions?thinking=true&reasoning_format=reasoning_content", nil)
	c.Request = req

	// Fake upstream http.Response
	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(b))),
	}

	err, _ := Handler(c, upstream, 0, "gpt-4")
	require.Nil(t, err, "unexpected error")

	// Decode written JSON
	var out SlimTextResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), "failed to unmarshal out")

	require.Len(t, out.Choices, 1, "expected 1 choice")
	msg := out.Choices[0].Message

	// Expect content cleaned of think tags
	require.Equal(t, "before  after", msg.StringContent(), "unexpected cleaned content")
	// Expect reasoning mapped to reasoning_content field
	require.NotNil(t, msg.ReasoningContent, "expected reasoning_content to be set")
	require.Equal(t, "xyz", *msg.ReasoningContent, "expected reasoning_content=xyz")
}

// TestHandler_NonStream_ReasoningFormatThinking ensures reasoning_content is remapped when thinking format requested.
func TestHandler_NonStream_ReasoningFormatThinking(t *testing.T) {
	t.Parallel()
	reasoning := "walkthrough"
	respStruct := SlimTextResponse{
		Choices: []TextResponseChoice{
			{
				Index: 0,
				Message: model.Message{
					Role:             "assistant",
					Content:          "1+1=2",
					ReasoningContent: &reasoning,
				},
				FinishReason: "stop",
			},
		},
		Usage: modelUsage(2, 3),
	}
	body, _ := json.Marshal(respStruct)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?thinking=true&reasoning_format=thinking", nil)
	c.Request = req

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}

	handlerErr, _ := Handler(c, upstream, 0, "kimi-k2-thinking")
	require.Nil(t, handlerErr, "unexpected error")

	var out SlimTextResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), "failed to decode handler output")
	require.Len(t, out.Choices, 1, "expected single choice")
	msg := out.Choices[0].Message
	require.NotNil(t, msg.Thinking, "expected thinking field to be set")
	require.Equal(t, "walkthrough", *msg.Thinking, "expected thinking field to carry reasoning")
	require.Nil(t, msg.ReasoningContent, "expected reasoning_content cleared")
}

// TestHandler_NonStream_OmitsEmptyErrorField verifies that the handler does not emit the error field
// when upstream responses omit it, preserving OpenAI compatibility for clients that gate on its presence.
func TestHandler_NonStream_OmitsEmptyErrorField(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request = req

	respStruct := SlimTextResponse{
		Choices: []TextResponseChoice{
			{
				Index:        0,
				Message:      structToMessage("plain response"),
				FinishReason: "stop",
			},
		},
		Usage: modelUsage(5, 7),
	}
	b, _ := json.Marshal(respStruct)

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(b))),
	}

	handlerErr, _ := Handler(c, upstream, 0, "gpt-4")
	require.Nil(t, handlerErr, "unexpected error")

	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), "failed to decode handler output")
	_, exists := out["error"]
	require.False(t, exists, "expected no error field in handler output, got %s", w.Body.String())
}

// Helpers
func structToMessage(s string) model.Message {
	return model.Message{Content: s, Role: "assistant"}
}

func modelUsage(p, c int) model.Usage {
	return model.Usage{PromptTokens: p, CompletionTokens: c, TotalTokens: p + c}
}
