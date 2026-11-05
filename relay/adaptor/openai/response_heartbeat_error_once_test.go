package openai

import (
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

type heartbeatFailureWriter struct {
	gin.ResponseWriter
	released chan struct{}
	once     sync.Once
	fault    error
}

// Write fails a heartbeat and releases the waiting upstream reader.
func (w *heartbeatFailureWriter) Write(p []byte) (int, error) {
	if strings.HasPrefix(string(p), ":") {
		w.once.Do(func() { close(w.released) })
		return 0, w.fault
	}
	return w.ResponseWriter.Write(p)
}

type heartbeatBlockedBody struct {
	released <-chan struct{}
	sent     bool
	closed   bool
	fault    error
}

// Read waits until heartbeat delivery fails, then returns one benign event.
func (b *heartbeatBlockedBody) Read(p []byte) (int, error) {
	if b.sent {
		return 0, io.EOF
	}
	<-b.released
	b.sent = true
	if b.fault != nil {
		return 0, b.fault
	}
	return copy(p, "event: heartbeat-test\n"), nil
}

// Close records upstream cleanup.
func (b *heartbeatBlockedBody) Close() error { b.closed = true; return nil }

// TestResponseHeartbeatErrorIsReturnedOnce verifies a failed heartbeat is returned without logging the same error.
func TestResponseHeartbeatErrorIsReturnedOnce(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler func(*gin.Context, *http.Response, int) (*model.ErrorWithStatusCode, string, *model.Usage)
		eof     bool
	}{{"native", ResponseAPIDirectStreamHandler, false}, {"converted", ResponseAPIStreamHandler, false}, {"native EOF", ResponseAPIDirectStreamHandler, true}, {"converted EOF", ResponseAPIStreamHandler, true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base, err := glog.NewConsoleWithName("heartbeat", glog.LevelDebug)
			require.NoError(t, err)
			core, logs := observer.New(zapcore.DebugLevel)
			logger := base.WithOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			gmw.SetLogger(c, logger)
			fault := stderrors.New("heartbeat write failure")
			released := make(chan struct{})
			c.Writer = &heartbeatFailureWriter{ResponseWriter: c.Writer, released: released, fault: fault}
			body := &heartbeatBlockedBody{released: released}
			if tc.eof {
				body.fault = io.EOF
			}
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
			apiErr, _, _ := tc.handler(c, resp, 0)
			require.NotNil(t, apiErr)
			require.ErrorIs(t, apiErr.RawError, fault)
			require.True(t, body.closed)
			entries := logs.FilterMessage("heartbeat diagnostics").All()
			require.Len(t, entries, 1)
			fields := entries[0].ContextMap()
			require.NotContains(t, fields, "heartbeat_write_err")
			require.Equal(t, true, fields["heartbeat_write_failed"])
		})
	}
}

// TestResponseHeartbeatSecondaryErrorRemainsLogged verifies an independent heartbeat failure remains visible when an upstream read error is returned.
func TestResponseHeartbeatSecondaryErrorRemainsLogged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler func(*gin.Context, *http.Response, int) (*model.ErrorWithStatusCode, string, *model.Usage)
	}{{"native", ResponseAPIDirectStreamHandler}, {"converted", ResponseAPIStreamHandler}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base, err := glog.NewConsoleWithName("heartbeat-secondary", glog.LevelDebug)
			require.NoError(t, err)
			core, logs := observer.New(zapcore.DebugLevel)
			logger := base.WithOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			gmw.SetLogger(c, logger)
			heartbeatErr := stderrors.New("heartbeat failure")
			upstreamErr := stderrors.New("upstream read failure")
			released := make(chan struct{})
			c.Writer = &heartbeatFailureWriter{ResponseWriter: c.Writer, released: released, fault: heartbeatErr}
			body := &heartbeatBlockedBody{released: released, fault: upstreamErr}
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
			apiErr, _, _ := tc.handler(c, resp, 0)
			require.NotNil(t, apiErr)
			require.ErrorIs(t, apiErr.RawError, upstreamErr)
			require.True(t, body.closed)
			entries := logs.FilterMessage("heartbeat diagnostics").All()
			require.Len(t, entries, 1)
			fields := entries[0].ContextMap()
			require.Equal(t, heartbeatErr.Error(), fields["heartbeat_write_err"])
			require.Equal(t, true, fields["heartbeat_write_failed"])
		})
	}
}
