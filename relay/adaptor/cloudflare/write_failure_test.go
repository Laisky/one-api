package cloudflare

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingResponseWriter struct {
	gin.ResponseWriter
	short bool
}

// Write simulates a disconnect or short write after response headers are committed.
func (w *failingResponseWriter) Write(p []byte) (int, error) {
	w.ResponseWriter.WriteHeaderNow()
	if w.short {
		return len(p) - 1, nil
	}
	return 0, io.ErrClosedPipe
}

// TestResponseWriteFailureRetainsUsage preserves calculated usage for already completed upstream work.
func TestResponseWriteFailureRetainsUsage(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(map[bool]string{false: "disconnect", true: "short"}[short], func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Writer = &failingResponseWriter{ResponseWriter: c.Writer, short: short}
			failure, usage := Handler(c, &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`))}, 3, "fixture")
			require.NotNil(t, failure)
			require.Equal(t, "write_response_body_failed", failure.Code)
			require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
			cause := io.ErrClosedPipe
			if short {
				cause = io.ErrShortWrite
			}
			require.ErrorIs(t, failure.RawError, cause)
			require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "cloudflare.Handler")
			require.Equal(t, http.StatusCreated, rec.Code)
			require.True(t, c.Writer.Written())
			require.NotNil(t, usage)
			require.Equal(t, 3, usage.PromptTokens)
			require.Positive(t, usage.CompletionTokens)
			require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
		})
	}
}
