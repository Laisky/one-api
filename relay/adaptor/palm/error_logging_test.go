package palm

import (
	stderrors "errors"
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
)

// failingBody simulates a stream read failure and optionally a close failure.
type failingBody struct{ readErr, closeErr error }

// Read returns the configured read failure without producing data.
func (b failingBody) Read([]byte) (int, error) {
	if b.readErr == nil {
		return 0, io.EOF
	}
	return 0, b.readErr
}

// Close returns the configured close failure.
func (b failingBody) Close() error { return b.closeErr }

// TestStreamHandlerErrorReporting verifies stream errors are returned once,
// retain their cause, and preserve the existing close-error precedence.
func TestStreamHandlerErrorReporting(t *testing.T) {
	readErr := stderrors.New("read failed")
	closeErr := stderrors.New("close failed")
	cases := []struct {
		name          string
		body          io.ReadCloser
		code          string
		cause         error
		wantInnerLogs int
	}{
		{"invalid JSON", io.NopCloser(strings.NewReader("{")), "unmarshal_response_body_failed", nil, 0},
		{"read failure", failingBody{readErr: readErr}, "read_response_body_failed", readErr, 0},
		{"read and close failure", failingBody{readErr: readErr, closeErr: closeErr}, "close_response_body_failed", closeErr, 1},
		{"close failure", failingBody{closeErr: closeErr}, "close_response_body_failed", closeErr, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewWithName("palm-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/", nil)
			gmw.SetLogger(c, lg)
			result, text := StreamHandler(c, &http.Response{Body: tc.body, Header: make(http.Header)})
			require.Empty(t, text)
			require.NotNil(t, result)
			require.Equal(t, tc.code, result.Code)
			require.Equal(t, http.StatusInternalServerError, result.StatusCode)
			require.Contains(t, fmt.Sprintf("%+v", result.RawError), "StreamHandler")
			if tc.cause != nil {
				require.ErrorIs(t, result.RawError, tc.cause)
			}
			require.Len(t, logs.FilterLevelExact(zapcore.ErrorLevel).All(), tc.wantInnerLogs)
		})
	}
}
