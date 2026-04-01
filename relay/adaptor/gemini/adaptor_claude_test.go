package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

// TestConvertClaudeRequest_PromotesStructuredOutputToResponseSchema verifies Claude structured requests use Gemini responseSchema instead of tool calling.
// Parameters: t coordinates the test execution.
// Returns: no values.
func TestConvertClaudeRequest_PromotesStructuredOutputToResponseSchema(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	adaptor := &Adaptor{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	req := &model.ClaudeRequest{
		Model:     "gemini-2.5-flash",
		MaxTokens: 256,
		Messages: []model.ClaudeMessage{{
			Role: "user",
			Content: []any{map[string]any{
				"type": "text",
				"text": "Provide a JSON object with fields topic and confidence for AI adoption in enterprises.",
			}},
		}},
		Tools: []model.ClaudeTool{{
			Name:        "topic_classifier",
			Description: "Return structured topic and confidence data",
			InputSchema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"topic":      map[string]any{"type": "string"},
					"confidence": map[string]any{"type": "number"},
				},
				"required": []string{"topic", "confidence"},
			},
		}},
		ToolChoice: map[string]any{"type": "tool", "name": "topic_classifier"},
	}

	converted, err := adaptor.ConvertClaudeRequest(ctx, req)
	require.NoError(t, err)

	geminiReq, ok := converted.(*ChatRequest)
	require.True(t, ok, "expected ChatRequest, got %T", converted)
	require.NotNil(t, geminiReq.GenerationConfig.ResponseSchema)
	require.Equal(t, "application/json", geminiReq.GenerationConfig.ResponseMimeType)
	require.Empty(t, geminiReq.Tools)
	require.Nil(t, geminiReq.ToolConfig)

	responseSchemaJSON, err := json.Marshal(geminiReq.GenerationConfig.ResponseSchema)
	require.NoError(t, err)
	require.Contains(t, string(responseSchemaJSON), "topic")
	require.Contains(t, string(responseSchemaJSON), "confidence")
	if value, ok := geminiReq.GenerationConfig.ResponseSchema.(map[string]any); ok {
		require.Equal(t, "OBJECT", value["type"])
	}
}

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

// TestConvertStreamingToClaudeResponse_OversizedChunk verifies oversized Gemini SSE lines convert to Claude SSE.
func TestConvertStreamingToClaudeResponse_OversizedChunk(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	adaptor := &Adaptor{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	largeText := strings.Repeat("m", 128*1024)
	streamChunk := ChatResponse{
		Candidates: []ChatCandidate{{
			FinishReason: "STOP",
			Content:      ChatContent{Parts: []Part{{Text: largeText}}},
		}},
		UsageMetadata: &UsageMetadata{
			PromptTokenCount:     4,
			CandidatesTokenCount: 3,
			TotalTokenCount:      7,
		},
	}

	chunkBytes, err := json.Marshal(streamChunk)
	require.NoError(t, err)

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
	require.Nil(t, errResp)
	defer newResp.Body.Close()

	convertedBody, err := io.ReadAll(newResp.Body)
	require.NoError(t, err)

	converted := string(convertedBody)
	require.Contains(t, converted, largeText[:1024])
	require.Contains(t, converted, largeText[len(largeText)-1024:])
	require.Contains(t, converted, "data: [DONE]")
}

// TestConvertStreamingToClaudeResponse_StructuredJSON verifies Gemini JSON text streams remain visible after Claude conversion.
// Parameters: t coordinates the test execution.
// Returns: no values.
func TestConvertStreamingToClaudeResponse_StructuredJSON(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	adaptor := &Adaptor{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	streamChunk := ChatResponse{
		Candidates: []ChatCandidate{{
			FinishReason: "STOP",
			Content: ChatContent{
				Parts: []Part{{Text: `{"topic":"AI adoption in enterprises","confidence":0.95}`}},
			},
		}},
		UsageMetadata: &UsageMetadata{
			PromptTokenCount:     9,
			CandidatesTokenCount: 12,
			TotalTokenCount:      21,
		},
	}

	chunkBytes, err := json.Marshal(streamChunk)
	require.NoError(t, err)

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

	metaInfo := &meta.Meta{ActualModelName: "gemini-2.5-flash", PromptTokens: 9}

	newResp, errResp := adaptor.convertStreamingToClaudeResponse(ctx, resp, streamBuf.Bytes(), metaInfo)
	require.Nil(t, errResp)
	defer newResp.Body.Close()

	convertedBody, err := io.ReadAll(newResp.Body)
	require.NoError(t, err)

	converted := string(convertedBody)
	require.Contains(t, converted, "topic")
	require.Contains(t, converted, "confidence")
	require.Contains(t, converted, "text_delta")
	require.Contains(t, converted, "message_stop")
	require.Contains(t, converted, "data: [DONE]")
}
