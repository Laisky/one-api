package deepl

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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

// TestResponseWriteFailureRetainsUsage verifies delivery failures retain paid source-character usage.
func TestResponseWriteFailureRetainsUsage(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(map[bool]string{false: "disconnect", true: "short"}[short], func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Writer = &failingResponseWriter{ResponseWriter: c.Writer, short: short}
			a := &Adaptor{promptText: "你好"}
			usage, failure := a.DoResponse(c, &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"translations":[{"text":"translated","detected_source_language":"ZH"}]}`))}, &meta.Meta{ActualModelName: "deepl-en"})
			require.NotNil(t, failure)
			require.Equal(t, "write_response_body_failed", failure.Code)
			require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
			cause := io.ErrClosedPipe
			if short {
				cause = io.ErrShortWrite
			}
			require.ErrorIs(t, failure.RawError, cause)
			require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "deepl.Handler")
			require.Equal(t, http.StatusCreated, rec.Code)
			require.True(t, c.Writer.Written())
			require.Equal(t, 2, usage.PromptTokens)
			require.Equal(t, 2, usage.TotalTokens)
		})
	}
}
