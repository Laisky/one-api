package ali

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		{"embedding decode", "{", "unmarshal_response_body_failed", "EmbeddingHandler", nil, nil, EmbeddingHandler},
		{"embedding close", "{}", "close_response_body_failed", "EmbeddingHandler", nil, failure, EmbeddingHandler},
		{"stream close", "", "close_response_body_failed", "StreamHandler", nil, failure, StreamHandler},
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
				require.Equal(t, failure.Error(), result.Message)
			}
			require.Contains(t, fmt.Sprintf("%+v", result.RawError), tc.frame)
		})
	}
}

// aliFailingWriter simulates client disconnection on response delivery.
type aliFailingWriter struct {
	gin.ResponseWriter
	err error
}

// Write returns the configured delivery error without writing data.
func (w aliFailingWriter) Write([]byte) (int, error) { return 0, w.err }

// TestDirectHandlersReturnWriteFailure checks that direct response write errors
// preserve the cause and usage while successful responses keep their wire body.
func TestDirectHandlersReturnWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler func(*gin.Context, *http.Response) (*model.ErrorWithStatusCode, *model.Usage)
	}{{"chat", Handler}, {"embedding", EmbeddingHandler}} {
		t.Run(tc.name, func(t *testing.T) {
			failure := errors.New("synthetic client write failure")
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			c.Writer = aliFailingWriter{ResponseWriter: c.Writer, err: failure}
			result, usage := tc.handler(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))})
			require.NotNil(t, result)
			require.NotNil(t, usage)
			require.Equal(t, "write_response_body_failed", result.Code)
			require.Equal(t, http.StatusInternalServerError, result.StatusCode)
			require.Equal(t, failure.Error(), result.Message)
			require.ErrorIs(t, result.RawError, failure)
			require.Contains(t, fmt.Sprintf("%+v", result.RawError), "ali.")
		})
	}
}
