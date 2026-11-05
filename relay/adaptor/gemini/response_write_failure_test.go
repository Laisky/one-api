package gemini

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// geminiDeliveryWriter captures attempted response bytes and optionally fails client delivery.
type geminiDeliveryWriter struct {
	*httptest.ResponseRecorder
	cause     error
	attempted []byte
	writes    int
	partial   bool
}

// Write captures the payload and returns the selected delivery result.
func (w *geminiDeliveryWriter) Write(p []byte) (int, error) {
	w.writes++
	w.attempted = append([]byte(nil), p...)
	if w.cause == nil {
		return w.ResponseRecorder.Write(p)
	}
	if w.partial {
		n, err := w.ResponseRecorder.Write(p[:len(p)/2])
		if err != nil {
			return n, err
		}
		return n, w.cause
	}
	return 0, w.cause
}

// TestGeminiResponseDeliveryFailure preserves response data and billable usage when the client write fails.
func TestGeminiResponseDeliveryFailure(t *testing.T) {
	tests := []struct {
		name, body                string
		run                       func(*gin.Context, *http.Response) (*model.ErrorWithStatusCode, *model.Usage)
		prompt, completion, total int
	}{
		{"chat", `{"candidates":[{"content":{"role":"model","parts":[{"text":"answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}}`, func(c *gin.Context, r *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
			return Handler(c, r, 42, "gemini-2.5-flash")
		}, 7, 3, 10},
		{"embedding", `{"embeddings":[{"values":[0.1,0.2]}]}`, func(c *gin.Context, r *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
			return EmbeddingHandler(c, r, 42)
		}, 42, 0, 42},
	}
	for _, tc := range tests {
		for _, mode := range []string{"success", "failure", "partial"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				cause := stderrors.New("client write failed")
				writer := &geminiDeliveryWriter{ResponseRecorder: httptest.NewRecorder(), partial: mode == "partial"}
				if mode != "success" {
					writer.cause = cause
				}
				c, _ := gin.CreateTestContext(writer)
				c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
				c.Set(ctxkey.EmbeddingPromptTokensDetails, &model.UsagePromptTokensDetails{TextTokens: 42})
				resp := &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(tc.body))}
				failure, usage := tc.run(c, resp)
				require.NotNil(t, usage)
				require.Equal(t, tc.prompt, usage.PromptTokens)
				require.Equal(t, tc.completion, usage.CompletionTokens)
				require.Equal(t, tc.total, usage.TotalTokens)
				if tc.name == "embedding" {
					require.Equal(t, 42, usage.PromptTokensDetails.TextTokens)
				}
				require.True(t, c.Writer.Written())
				require.Equal(t, http.StatusAccepted, c.Writer.Status())
				require.Equal(t, http.StatusAccepted, writer.Code)
				require.Equal(t, "application/json", writer.Header().Get("Content-Type"))
				require.Equal(t, 1, writer.writes)
				require.True(t, json.Valid(writer.attempted))
				require.Contains(t, string(writer.attempted), `"usage"`)
				if mode == "success" {
					require.Nil(t, failure)
					require.Equal(t, writer.attempted, writer.Body.Bytes())
				} else {
					require.NotNil(t, failure)
					require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
					require.Equal(t, "write_response_body_failed", failure.Code)
					require.ErrorIs(t, failure.RawError, cause)
					require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "gemini.")
				}
			})
		}
	}
}

// TestGeminiAdaptorPreservesUsageOnDeliveryFailure verifies response errors and usage reach the billing caller together.
func TestGeminiAdaptorPreservesUsageOnDeliveryFailure(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		mode       int
		prompt     int
	}{
		{"chat", `{"candidates":[{"content":{"parts":[{"text":"answer"}]}}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}}`, relaymode.ChatCompletions, 7},
		{"embedding", `{"embeddings":[{"values":[0.1]}]}`, relaymode.Embeddings, 42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := stderrors.New("client disconnected")
			writer := &geminiDeliveryWriter{ResponseRecorder: httptest.NewRecorder(), cause: cause}
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body))}
			usage, failure := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{Mode: tc.mode, PromptTokens: 42, ActualModelName: "gemini-2.5-flash"})
			require.NotNil(t, usage)
			require.Equal(t, tc.prompt, usage.PromptTokens)
			require.NotNil(t, failure)
			require.ErrorIs(t, failure.RawError, cause)
			require.True(t, c.Writer.Written())
		})
	}
}
