package gemini

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
	"github.com/Laisky/one-api/relay/adaptor/internal/streambridgetest"
)

// geminiStreamSSE builds a canned Gemini streaming body that emits one
// `data: {ChatResponse}` frame per supplied text token followed by the terminal
// `data: [DONE]` sentinel, matching the wire format streamResponseGeminiChat2OpenAI
// parses in StreamHandler.
func geminiStreamSSE(t *testing.T, tokens []string) string {
	t.Helper()
	var b strings.Builder
	for _, tok := range tokens {
		chunk := ChatResponse{
			Candidates: []ChatCandidate{{
				Content: ChatContent{Parts: []Part{{Text: tok}}},
			}},
			UsageMetadata: &UsageMetadata{
				PromptTokenCount:     3,
				CandidatesTokenCount: 2,
				TotalTokenCount:      5,
			},
		}
		raw, err := json.Marshal(chunk)
		require.NoError(t, err)
		b.WriteString("data: " + string(raw) + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func newGeminiStreamResp(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// TestStreamHandler_RoutesThroughRewriter proves the bug-class fix: when the
// /v1/responses chat-fallback installs a stream-rewrite bridge, every Gemini
// stream token flows through the bridge (HandleChunk + FinalizeUsage + HandleDone)
// and NO raw chat.completion.chunk or "data: [DONE]" leaks to the client.
func TestStreamHandler_RoutesThroughRewriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	resp := newGeminiStreamResp(geminiStreamSSE(t, []string{"Hello", " world"}))

	apiErr, responseText, usage := StreamHandler(c, resp)
	require.Nil(t, apiErr)
	require.Equal(t, "Hello world", responseText)
	require.NotNil(t, usage)

	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount, "HandleDone must be called exactly once")
	require.True(t, rw.UsageSet, "FinalizeUsage must be invoked with computed usage")

	// The bridge fully intercepts output: nothing leaks to the recorder.
	body := rec.Body.String()
	require.NotContains(t, body, "chat.completion.chunk",
		"raw chat-completion chunk leaked despite active bridge; body:\n%s", body)
	require.NotContains(t, body, "data: [DONE]",
		"raw [DONE] sentinel leaked despite active bridge; body:\n%s", body)
}

// TestStreamHandler_NoBridgeEmitsChatChunks verifies the plain Chat Completions
// path is unchanged: without a rewrite bridge the handler still emits
// chat.completion.chunk SSE frames and a "data: [DONE]" sentinel.
func TestStreamHandler_NoBridgeEmitsChatChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := newGeminiStreamResp(geminiStreamSSE(t, []string{"Hello", " world"}))

	apiErr, responseText, _ := StreamHandler(c, resp)
	require.Nil(t, apiErr)
	require.Equal(t, "Hello world", responseText)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`,
		"expected chat.completion.chunk frames; body:\n%s", body)
	require.Contains(t, body, `"content":"Hello"`, "missing first content delta; body:\n%s", body)
	require.Contains(t, body, `"content":" world"`, "missing second content delta; body:\n%s", body)
	require.Contains(t, body, "data: [DONE]", "missing [DONE] sentinel; body:\n%s", body)
}

// TestStreamHandlerCountsOnlyDeliveredImages checks billed image counts against the emitted stream.
func TestStreamHandlerCountsOnlyDeliveredImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	chunk := ChatResponse{Candidates: []ChatCandidate{
		{Content: ChatContent{Parts: []Part{{InlineData: &InlineData{MimeType: "image/png", Data: "YQ=="}}, {InlineData: &InlineData{Data: "Yg=="}}}}},
		{Content: ChatContent{Parts: []Part{{InlineData: &InlineData{MimeType: "image/png", Data: "Yw=="}}}}},
	}}
	raw, err := json.Marshal(chunk)
	require.NoError(t, err)
	apiErr, _, _ := StreamHandler(c, newGeminiStreamResp("data: "+string(raw)+"\n\ndata: [DONE]\n\n"))
	require.Nil(t, apiErr)
	require.Contains(t, rec.Body.String(), "YQ==")
	require.NotContains(t, rec.Body.String(), "Yg==")
	require.NotContains(t, rec.Body.String(), "Yw==")
	require.Equal(t, 1, c.GetInt(ctxkey.OutputImageCount))
}

// TestStreamHandlerMalformedImagePartCountsDelivered verifies ambiguous parts do not incur an extra image fee.
func TestStreamHandlerMalformedImagePartCountsDelivered(t *testing.T) {
	for _, tc := range []struct {
		name            string
		part            Part
		count           int
		present, absent string
	}{
		{"inline", Part{InlineData: &InlineData{MimeType: "image/png", Data: "YQ=="}}, 1, "YQ==", "file.example"},
		{"file", Part{FileData: &FileData{MimeType: "image/png", FileURI: "https://file.example/a.png"}}, 1, "file.example", "YQ=="},
		{"both", Part{InlineData: &InlineData{MimeType: "image/png", Data: "YQ=="}, FileData: &FileData{MimeType: "image/png", FileURI: "https://file.example/a.png"}}, 1, "YQ==", "file.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			raw, err := json.Marshal(ChatResponse{Candidates: []ChatCandidate{{Content: ChatContent{Parts: []Part{tc.part}}}}})
			require.NoError(t, err)
			apiErr, _, _ := StreamHandler(c, newGeminiStreamResp("data: "+string(raw)+"\n\ndata: [DONE]\n\n"))
			require.Nil(t, apiErr)
			require.Contains(t, rec.Body.String(), tc.present)
			require.NotContains(t, rec.Body.String(), tc.absent)
			require.Equal(t, tc.count, c.GetInt(ctxkey.OutputImageCount))
		})
	}
}
