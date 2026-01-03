package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

func TestConvertNonStreamingToClaudeResponse_Basic(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	adaptor := &Adaptor{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	geminiResp := ChatResponse{
		Candidates: []ChatCandidate{
			{
				FinishReason: "STOP",
				Content: ChatContent{
					Parts: []Part{{Text: "Hello from Gemini"}},
				},
			},
		},
		UsageMetadata: &UsageMetadata{
			PromptTokenCount:     5,
			CandidatesTokenCount: 7,
			TotalTokenCount:      12,
		},
	}

	bodyBytes, err := json.Marshal(geminiResp)
	require.NoError(t, err, "marshal gemini response")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}
	resp.Header.Set("Content-Type", "application/json")

	metaInfo := &meta.Meta{ActualModelName: "gemini-2.5-flash", PromptTokens: 5}

	newResp, errResp := adaptor.convertNonStreamingToClaudeResponse(ctx, resp, bodyBytes, metaInfo)
	require.Nil(t, errResp, "convert non-streaming returned error")
	defer newResp.Body.Close()

	convertedBody, err := io.ReadAll(newResp.Body)
	require.NoError(t, err, "read converted body")

	var claudeResp model.ClaudeResponse
	require.NoError(t, json.Unmarshal(convertedBody, &claudeResp), "unmarshal claude response")

	require.NotEmpty(t, claudeResp.Content, "content should not be empty")
	require.Equal(t, "Hello from Gemini", claudeResp.Content[0].Text, "unexpected content")
	require.Equal(t, "end_turn", claudeResp.StopReason, "unexpected stop reason")
	require.Equal(t, 5, claudeResp.Usage.InputTokens, "unexpected input tokens")
	require.Equal(t, 7, claudeResp.Usage.OutputTokens, "unexpected output tokens")
}

func TestConvertStreamingToClaudeResponse_Basic(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	adaptor := &Adaptor{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	streamChunk := ChatResponse{
		Candidates: []ChatCandidate{
			{
				FinishReason: "STOP",
				Content: ChatContent{
					Parts: []Part{{Text: "Hello from Gemini"}},
				},
			},
		},
		UsageMetadata: &UsageMetadata{
			PromptTokenCount:     4,
			CandidatesTokenCount: 3,
			TotalTokenCount:      7,
		},
	}

	chunkBytes, err := json.Marshal(streamChunk)
	require.NoError(t, err, "marshal stream chunk")

	streamBuf := bytes.NewBuffer(nil)
	streamBuf.WriteString("data: ")
	streamBuf.Write(chunkBytes)
	streamBuf.WriteString("\n\n")
	streamBuf.WriteString("data: [DONE]\n\n")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(streamBuf.Bytes())),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	metaInfo := &meta.Meta{ActualModelName: "gemini-2.5-flash", PromptTokens: 4}

	newResp, errResp := adaptor.convertStreamingToClaudeResponse(ctx, resp, streamBuf.Bytes(), metaInfo)
	require.Nil(t, errResp, "convert streaming returned error")
	defer newResp.Body.Close()

	require.Equal(t, "text/event-stream", newResp.Header.Get("Content-Type"), "unexpected content type")

	convertedBody, err := io.ReadAll(newResp.Body)
	require.NoError(t, err, "read converted stream")

	converted := string(convertedBody)
	require.Contains(t, converted, "event: message_start", "missing message_start event")
	require.Contains(t, converted, "\"text_delta\"", "missing text delta")
	require.Contains(t, converted, "Hello from Gemini", "missing response text")
	require.Contains(t, converted, "\"input_tokens\":4", "missing usage input tokens")
	require.Contains(t, converted, "data: [DONE]", "missing done marker")
}
