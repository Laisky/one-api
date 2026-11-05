package logger

import (
	"context"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// observedLogger returns a glog logger whose output is captured in memory.
//
// Parameters:
//   - t: the test handle.
//   - name: the logger name, so a test can tell which logger wrote a line.
//
// Return values:
//   - glog.Logger: the logger.
//   - *observer.ObservedLogs: what the logger wrote.
func observedLogger(t *testing.T, name string) (glog.Logger, *observer.ObservedLogs) {
	t.Helper()

	base, err := glog.NewConsoleWithName(name, glog.LevelDebug)
	require.NoError(t, err)

	core, logs := observer.New(zapcore.DebugLevel)
	return base.WithOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })), logs
}

// installObservedProcessLogger replaces the process logger with an observed one
// for the duration of the test.
//
// Parameters:
//   - t: the test handle; the previous logger is restored on cleanup.
//
// Return values:
//   - *observer.ObservedLogs: what the process logger wrote.
func installObservedProcessLogger(t *testing.T) *observer.ObservedLogs {
	t.Helper()

	previous := Logger
	t.Cleanup(func() { Logger = previous })

	lg, logs := observedLogger(t, "process")
	Logger = lg
	return logs
}

// TestFromContextFallsBackToTheProcessLogger pins the documented contract of
// FromContext: a context that carries no request logger gets the process
// logger.
//
// It used to get gin-middlewares' library fallback instead, which is a separate
// stdout-only logger. Background workers that log through FromContext (MCP
// catalog sync, async tasks, billing reconciliation) were therefore missing
// from the log file, the OTLP export, sampling and alerting, while looking
// perfectly normal on the console.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestFromContextFallsBackToTheProcessLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name string
		ctx  func() context.Context
	}{
		{"nil context", func() context.Context { return nil }},
		{"background context", context.Background},
		{"cancellable process context", func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			return ctx
		}},
		{"gin context without a request logger", func() context.Context {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/", nil)
			return c
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := installObservedProcessLogger(t)

			FromContext(tc.ctx()).Info("background work finished")

			require.Equal(t, 1, logs.FilterMessage("background work finished").Len(),
				"a context without a request logger must log through the process logger")
		})
	}
}

// TestFromContextReturnsTheRequestLogger proves the fallback above did not
// break the primary path: a request logger stored with gin-middlewares' public
// SetLogger is returned for a std context, a gin context, and a std context
// derived from a gin context.
//
// It also pins the context key FromContext relies on. gin-middlewares does not
// export it, so if that library ever renames it, these cases fail instead of
// request logs silently losing their request fields.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestFromContextReturnsTheRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name string
		ctx  func(glog.Logger) context.Context
	}{
		{"std context", func(lg glog.Logger) context.Context {
			return gmw.SetLogger(context.Background(), lg)
		}},
		{"gin context", func(lg glog.Logger) context.Context {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/", nil)
			gmw.SetLogger(c, lg)
			return c
		}},
		{"std context derived from a gin context", func(lg glog.Logger) context.Context {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/", nil)
			gmw.SetLogger(c, lg)
			return gmw.Ctx(c)
		}},
		{"detached std context", func(lg glog.Logger) context.Context {
			return context.WithoutCancel(gmw.SetLogger(context.Background(), lg))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			processLogs := installObservedProcessLogger(t)
			requestLogger, requestLogs := observedLogger(t, "request")

			FromContext(tc.ctx(requestLogger)).Info("relay finished")

			require.Equal(t, 1, requestLogs.FilterMessage("relay finished").Len(),
				"a context carrying a request logger must log through it")
			require.Zero(t, processLogs.Len(), "the process logger must not receive request lines")
		})
	}
}
