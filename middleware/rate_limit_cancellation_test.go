package middleware

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
)

// limiterContextResult isolates a dependency cancellation with a still-live request.
type limiterContextResult struct {
	redis.Cmdable
	err error
}

// LLen returns a controlled dependency error without changing request lifecycle state.
func (r limiterContextResult) LLen(context.Context, string) *redis.IntCmd {
	return redis.NewIntResult(0, r.err)
}

// limiterLog creates a local structured log sink for one synthetic request.
func limiterLog(t *testing.T) (glog.Logger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "abort.json")
	logger, err := glog.New(glog.WithName("limiter-cancellation-test"), glog.WithLevel(glog.LevelInfo), glog.WithEncoding(glog.EncodingJSON), glog.WithOutputPaths([]string{path}), glog.WithErrorOutputPaths([]string{path}))
	require.NoError(t, err)
	return logger, path
}

// readLimiterEvent flushes and reads exactly one synthetic abort record.
func readLimiterEvent(t *testing.T, logger glog.Logger, path string) map[string]any {
	t.Helper()
	require.NoError(t, logger.Sync())
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var event map[string]any
	require.NoError(t, json.Unmarshal(raw, &event))
	return event
}

// TestRedisRateLimitRequestCancellation exercises the real limiter and Redis client.
func TestRedisRateLimitRequestCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, name := range []string{"healthy", "request_canceled_healthy_redis", "redis_server_error", "redis_connection_refused", "dependency_canceled_live_request", "dependency_deadline_live_request"} {
		t.Run(name, func(t *testing.T) {
			previous := common.RDB
			t.Cleanup(func() { common.RDB = previous })
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			common.RDB = client
			switch name {
			case "redis_server_error":
				server.SetError("ERR synthetic Redis failure")
			case "redis_connection_refused":
				server.Close()
			case "dependency_canceled_live_request":
				common.RDB = limiterContextResult{Cmdable: client, err: context.Canceled}
			case "dependency_deadline_live_request":
				common.RDB = limiterContextResult{Cmdable: client, err: context.DeadlineExceeded}
			}
			logger, logPath := limiterLog(t)
			engine := gin.New()
			engine.Use(RequestLifecycle(), func(c *gin.Context) { gmw.SetLogger(c, logger); c.Next() })
			dispatched := false
			engine.GET("/api/browser-config", func(c *gin.Context) { redisRateLimiter(c, 100, 60, "GA") }, func(c *gin.Context) { dispatched = true; c.Status(http.StatusOK) })
			request := httptest.NewRequest(http.MethodGet, "/api/browser-config", nil)
			if name == "request_canceled_healthy_redis" {
				ctx, cancel := context.WithCancel(request.Context())
				cancel()
				request = request.WithContext(ctx)
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if name == "healthy" {
				require.Equal(t, http.StatusOK, response.Code)
				require.True(t, dispatched)
				require.Equal(t, 3, server.CommandCount(), "retain LLEN, LPUSH and expiry on successful admission")
				require.NoError(t, logger.Sync())
				raw, err := os.ReadFile(logPath)
				require.NoError(t, err)
				require.Empty(t, raw)
				return
			}
			require.Equal(t, http.StatusInternalServerError, response.Code)
			require.False(t, dispatched, "failed admission must never reach the controller")
			event := readLimiterEvent(t, logger, logPath)
			require.Equal(t, "server abort", event["message"])
			require.EqualValues(t, 500, event["status_code"])
			if name == "request_canceled_healthy_redis" {
				require.Equal(t, "WARN", event["level"])
				require.Equal(t, "request_canceled", event["error_kind"])
				require.Equal(t, "failed to get list length: context canceled", event["error"])
				require.NotContains(t, event, "errorVerbose")
				require.Zero(t, server.CommandCount(), "canceled request must not require a Redis outage or command")
				require.Contains(t, response.Body.String(), "failed to get list length: context canceled")
			} else {
				require.Equal(t, "ERROR", event["level"])
				require.Equal(t, "unknown", event["error_kind"])
				switch name {
				case "redis_server_error":
					require.Contains(t, event["error"], "ERR synthetic Redis failure")
				case "redis_connection_refused":
					require.Contains(t, event["error"], "connection refused")
				case "dependency_canceled_live_request":
					require.Equal(t, "failed to get list length: context canceled", event["error"])
				case "dependency_deadline_live_request":
					require.Equal(t, "failed to get list length: context deadline exceeded", event["error"])
				}
			}
		})
	}
}

// TestRedisRateLimitTCPDisconnect turns the alert's generic failure mode into a regression.
func TestRedisRateLimitTCPDisconnect(t *testing.T) {
	previous := common.RDB
	t.Cleanup(func() { common.RDB = previous })
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr(), MaxRetries: -1})
	common.RDB = client
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	logger, logPath := limiterLog(t)
	entered, ended := make(chan struct{}), make(chan int, 1)
	engine := gin.New()
	engine.Use(RequestLifecycle())
	engine.GET("/api/browser-config", func(c *gin.Context) {
		gmw.SetLogger(c, logger)
		close(entered)
		// Synchronize cancellation before LLEN; this proves sufficiency, not incident timing.
		select {
		case <-c.Request.Context().Done():
		case <-time.After(3 * time.Second):
			ended <- -1
			return
		}
		redisRateLimiter(c, 100, 60, "GA")
		ended <- c.Writer.Status()
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	connection, err := net.Dial("tcp", server.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	_, err = connection.Write([]byte("GET /api/browser-config HTTP/1.1\r\nHost: synthetic.invalid\r\n\r\n"))
	require.NoError(t, err)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not enter handler")
	}
	require.NoError(t, connection.Close())
	select {
	case status := <-ended:
		require.Equal(t, http.StatusInternalServerError, status, "preserve attempted status; peer has already closed")
	case <-time.After(4 * time.Second):
		t.Fatal("TCP close did not cancel request")
	}
	event := readLimiterEvent(t, logger, logPath)
	require.Equal(t, "WARN", event["level"])
	require.Equal(t, "request_canceled", event["error_kind"])
	require.Equal(t, "failed to get list length: context canceled", event["error"])
	require.NotContains(t, event, "errorVerbose")
	require.Zero(t, redisServer.CommandCount())
}
