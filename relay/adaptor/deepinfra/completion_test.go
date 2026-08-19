package deepinfra

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

func TestHandleCompletionResponsePreservesTextEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(bytes.NewBufferString(`{
			"id":"cmpl-1",
			"object":"text_completion",
			"model":"deepseek-ai/DeepSeek-V4-Flash",
			"choices":[{"text":"hello world","index":0,"finish_reason":"stop","logprobs":null}]
		}`)),
	}

	relayError, usage := handleCompletionResponse(context, response, 5, "deepseek-ai/DeepSeek-V4-Flash")
	require.Nil(t, relayError)
	require.Equal(t, 5, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, 7, usage.TotalTokens)

	var rendered map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &rendered))
	var choices []struct {
		Text         string          `json:"text"`
		Index        int             `json:"index"`
		FinishReason string          `json:"finish_reason"`
		Logprobs     json.RawMessage `json:"logprobs"`
	}
	require.NoError(t, json.Unmarshal(rendered["choices"], &choices))
	require.Len(t, choices, 1)
	require.Equal(t, "hello world", choices[0].Text)
	require.Equal(t, "stop", choices[0].FinishReason)

	var renderedUsage model.Usage
	require.NoError(t, json.Unmarshal(rendered["usage"], &renderedUsage))
	require.Equal(t, 7, renderedUsage.TotalTokens)
}

func TestHandleCompletionResponseUsesUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(bytes.NewBufferString(`{
			"choices":[{"text":"hello","index":0}],
			"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}
		}`)),
	}

	relayError, usage := handleCompletionResponse(context, response, 5, "model")
	require.Nil(t, relayError)
	require.Equal(t, 11, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Equal(t, 14, usage.TotalTokens)
}

func TestHandleCompletionStreamPreservesSSEAndSynthesizesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	body := "data: {\"id\":\"cmpl-1\",\"choices\":[{\"text\":\"hello \"}]}\n\n" +
		"data: {\"id\":\"cmpl-1\",\"choices\":[{\"text\":\"world\"}]}\n\n" +
		"data: [DONE]\n\n"
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}

	relayError, usage := handleCompletionStream(context, response, 4, "model")
	require.Nil(t, relayError)
	require.Equal(t, body, recorder.Body.String())
	require.Equal(t, 4, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, 6, usage.TotalTokens)
}

func TestHandleCompletionStreamUsesUsageChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	body := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":4,\"total_tokens\":13}}\n\n" +
		"data: [DONE]\n\n"
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}

	relayError, usage := handleCompletionStream(context, response, 1, "model")
	require.Nil(t, relayError)
	require.Equal(t, 9, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 13, usage.TotalTokens)
}
