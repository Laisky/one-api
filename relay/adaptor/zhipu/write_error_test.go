package zhipu

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// zhipuFailingWriter simulates an error delivering the upstream response.
type zhipuFailingWriter struct {
	gin.ResponseWriter
	err error
}

// Write returns the injected client delivery failure.
func (w zhipuFailingWriter) Write([]byte) (int, error) { return 0, w.err }

// TestDirectHandlersReturnWriteFailure checks failed response writes retain
// usage, error identity, response code, and returning function stack frame.
func TestDirectHandlersReturnWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name, body, frame string
		handler           func(*gin.Context, *http.Response) (*model.ErrorWithStatusCode, *model.Usage)
	}{
		{"chat", `{"success":true,"data":{"usage":{"total_tokens":11}}}`, "zhipu.Handler", Handler},
		{"embedding", `{"usage":{"total_tokens":11}}`, "zhipu.EmbeddingsHandler", EmbeddingsHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := errors.New("synthetic client write failure")
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Writer = zhipuFailingWriter{ResponseWriter: c.Writer, err: failure}
			result, usage := tc.handler(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))})
			require.NotNil(t, result)
			require.NotNil(t, usage)
			require.Equal(t, 11, usage.TotalTokens)
			require.Equal(t, "write_response_body_failed", result.Code)
			require.Equal(t, http.StatusInternalServerError, result.StatusCode)
			require.Equal(t, failure.Error(), result.Message)
			require.ErrorIs(t, result.RawError, failure)
			require.Contains(t, fmt.Sprintf("%+v", result.RawError), tc.frame)
		})
	}
}
