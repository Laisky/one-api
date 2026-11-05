package aiproxy

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// originBody exposes configured external read and close failures.
type originBody struct {
	reader            io.Reader
	readErr, closeErr error
}

// Read reads the configured body or returns the configured failure.
func (b originBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.reader.Read(p)
}

// Close returns the configured close failure.
func (b originBody) Close() error { return b.closeErr }

// TestExternalErrorsCarryOrigin checks externally originated errors preserve API
// codes, messages, causes, and a stack frame identifying the returning function.
func TestExternalErrorsCarryOrigin(t *testing.T) {
	failure := errors.New("synthetic external failure")
	cases := []struct {
		name, body, code, frame string
		readErr, closeErr       error
		invoke                  func(*gin.Context, *http.Response) (*model.ErrorWithStatusCode, *model.Usage)
	}{
		{"handler read", "", "read_response_body_failed", "Handler", failure, nil, Handler},
		{"handler close", "{}", "close_response_body_failed", "Handler", nil, failure, Handler},
		{"handler json", "{", "unmarshal_response_body_failed", "Handler", nil, nil, Handler},
		{"stream close", "", "close_response_body_failed", "StreamHandler", nil, failure, StreamHandler},
		{"stream read", "", "read_stream_failed", "LineReader", failure, nil, StreamHandler},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			result, _ := tc.invoke(c, &http.Response{Body: originBody{reader: strings.NewReader(tc.body), readErr: tc.readErr, closeErr: tc.closeErr}, Header: make(http.Header)})
			require.NotNil(t, result)
			require.Equal(t, tc.code, result.Code)
			require.Equal(t, http.StatusInternalServerError, result.StatusCode)
			if tc.readErr != nil || tc.closeErr != nil {
				require.ErrorIs(t, result.RawError, failure)
				require.Contains(t, result.Message, failure.Error())
			}
			require.Contains(t, fmt.Sprintf("%+v", result.RawError), tc.frame)
		})
	}
}

// TestStreamReadAndCloseFailure preserves close-error precedence while recording
// the separate read error that cannot be returned to the caller.
func TestStreamReadAndCloseFailure(t *testing.T) {
	readErr, closeErr := errors.New("synthetic read failure secret-token-123"), errors.New("synthetic close failure")
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("aiproxy-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	gmw.SetLogger(c, lg)
	result, usage := StreamHandler(c, &http.Response{Body: originBody{readErr: readErr, closeErr: closeErr}, Header: make(http.Header)})
	require.NotNil(t, result)
	require.Nil(t, usage)
	require.Equal(t, "close_response_body_failed", result.Code)
	require.Equal(t, closeErr.Error(), result.Message)
	require.ErrorIs(t, result.RawError, closeErr)
	entries := logs.FilterMessage("error reading stream response").All()
	require.Len(t, entries, 1)
	require.Equal(t, "[REDACTED]", entries[0].ContextMap()["error"])
	require.Equal(t, zapcore.WarnLevel, entries[0].Level)
}

// failingResponseWriter injects a client write failure into the handler.
type failingResponseWriter struct {
	gin.ResponseWriter
	err error
}

// Write returns the configured failure without writing a response body.
func (w failingResponseWriter) Write([]byte) (int, error) { return 0, w.err }

// TestHandlerWriteFailureCarriesOrigin verifies response delivery failures
// retain the error contract and origin stack for upstream callers.
func TestHandlerWriteFailureCarriesOrigin(t *testing.T) {
	failure := errors.New("synthetic write failure")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resultOK, usageOK := Handler(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))})
	require.Nil(t, resultOK)
	require.NotNil(t, usageOK)
	c.Writer = failingResponseWriter{ResponseWriter: c.Writer, err: failure}
	result, usage := Handler(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))})
	require.Equal(t, usageOK, usage)
	require.NotNil(t, result)
	require.Equal(t, "write_response_body_failed", result.Code)
	require.Equal(t, failure.Error(), result.Message)
	require.ErrorIs(t, result.RawError, failure)
	require.Contains(t, fmt.Sprintf("%+v", result.RawError), "Handler")
}
