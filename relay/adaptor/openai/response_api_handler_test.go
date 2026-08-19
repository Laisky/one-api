package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
)

// newGinTestContext creates a gin context and recorder for handler tests.
func newGinTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request = req

	gmw.SetLogger(ctx, logger.Logger)
	ctx.Set(ctxkey.RequestId, "req-test")

	return ctx, recorder
}

// TestResponseAPIHandlerRewritesHeaders verifies converted responses expose accurate length and remove unsupported encodings.
func TestResponseAPIHandlerRewritesHeaders(t *testing.T) {
	ctx, recorder := newGinTestContext(t)

	response := ResponseAPIResponse{
		Id:        "resp_test",
		Object:    "response",
		CreatedAt: 1,
		Status:    "completed",
		Model:     "gpt-5-nano",
		Output: []OutputItem{
			{
				Type:   "message",
				Role:   "assistant",
				Status: "completed",
				Content: []OutputContent{
					{Type: "output_text", Text: "hello"},
				},
			},
		},
		Usage: &ResponseAPIUsage{
			InputTokens:  4,
			OutputTokens: 3,
			TotalTokens:  7,
		},
	}

	raw, err := json.Marshal(response)
	require.NoError(t, err)

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Content-Length", "9999")
	upstream.Header.Set("Content-Encoding", "gzip")
	upstream.Header.Set("Transfer-Encoding", "chunked")
	upstream.Header.Set("X-Upstream", "ok")

	errResp, usage := ResponseAPIHandler(ctx, upstream, 2, "gpt-5-nano")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "ok", recorder.Header().Get("X-Upstream"))
	require.Empty(t, recorder.Header().Get("Content-Encoding"))
	require.Empty(t, recorder.Header().Get("Transfer-Encoding"))
	require.Equal(t, strconv.Itoa(len(recorder.Body.Bytes())), recorder.Header().Get("Content-Length"))
	require.NotEmpty(t, recorder.Body.String())
}

// TestResponseAPIDirectHandlerRewritesHeaders ensures direct pass-through responses expose correct headers after buffering.
func TestResponseAPIDirectHandlerRewritesHeaders(t *testing.T) {
	ctx, recorder := newGinTestContext(t)

	raw := []byte(`{"id":"resp_test","object":"response","created_at":1,"status":"completed","model":"gpt-5-nano","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7}}`)

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Content-Length", "9999")
	upstream.Header.Set("Content-Encoding", "gzip")
	upstream.Header.Set("X-Upstream", "direct")

	errResp, usage := ResponseAPIDirectHandler(ctx, upstream, 2, "gpt-5-nano")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "direct", recorder.Header().Get("X-Upstream"))
	require.Empty(t, recorder.Header().Get("Content-Encoding"))
	require.Equal(t, strconv.Itoa(len(recorder.Body.Bytes())), recorder.Header().Get("Content-Length"))
	require.JSONEq(t, string(raw), recorder.Body.String())
}

func TestResponseAPIDirectHandlerWebSearchUsageFallback(t *testing.T) {
	ctx, recorder := newGinTestContext(t)

	raw := []byte(`{"id":"resp_ws","object":"response","created_at":1,"status":"completed","model":"gpt-5-mini","output":[{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6,"input_tokens_details":{"web_search":{"requests":2}}}}`)

	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}
	upstream.Header.Set("Content-Type", "application/json")

	errResp, usage := ResponseAPIDirectHandler(ctx, upstream, 2, "gpt-5-mini")
	require.Nil(t, errResp)
	require.NotNil(t, usage)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, string(raw), recorder.Body.String())

	countRaw, exists := ctx.Get(ctxkey.WebSearchCallCount)
	require.True(t, exists)
	require.Equal(t, 2, countRaw.(int))
}
