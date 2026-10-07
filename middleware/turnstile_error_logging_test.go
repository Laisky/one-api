package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
)

// turnstileOfflineTransport intercepts verification requests without opening a network connection.
type turnstileOfflineTransport struct {
	calls int
	body  string
	err   error
}

// RoundTrip counts the intercepted request and returns a synthetic response or transport failure.
func (s *turnstileOfflineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}

// installTurnstileOfflineTransport enables the check and restores its global configuration and client after the test.
func installTurnstileOfflineTransport(t *testing.T, transport *turnstileOfflineTransport) {
	t.Helper()
	previousEnabled, previousClient := config.TurnstileCheckEnabled, http.DefaultClient
	config.TurnstileCheckEnabled = true
	http.DefaultClient = &http.Client{Transport: transport}
	t.Cleanup(func() {
		config.TurnstileCheckEnabled, http.DefaultClient = previousEnabled, previousClient
	})
}

// TestVerifyTurnstileTokenEmptyClassification verifies the exact client error without making a verification request.
func TestVerifyTurnstileTokenEmptyClassification(t *testing.T) {
	transport := &turnstileOfflineTransport{}
	installTurnstileOfflineTransport(t, transport)
	err := VerifyTurnstileToken("", "192.0.2.1")
	require.EqualError(t, err, "Turnstile token is empty")
	require.Zero(t, transport.calls)
	require.Equal(t, errkind.InvalidRequest, errkind.Of(err))
}

// TestTurnstileCheckErrorLogging verifies real middleware rejection, the helper response contract and fault-specific severity offline.
func TestTurnstileCheckErrorLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, query, body, message string
		transportError             error
		calls                      int
		level                      zapcore.Level
		kind                       string
	}{
		{name: "missing_token", query: "?email=", message: "Turnstile token is empty", level: zapcore.WarnLevel, kind: "invalid_request"},
		{name: "empty_token", query: "?email=&turnstile=", message: "Turnstile token is empty", level: zapcore.WarnLevel, kind: "invalid_request"},
		{name: "transport_failure", query: "?email=&turnstile=offline-challenge", transportError: errors.New("offline transport failure"), message: "turnstile check request failed", calls: 1, level: zapcore.ErrorLevel, kind: "unknown"},
		{name: "decode_failure", query: "?email=&turnstile=offline-challenge", body: "{", message: "turnstile response decode failed", calls: 1, level: zapcore.ErrorLevel, kind: "unknown"},
		{name: "verification_failure", query: "?email=&turnstile=offline-challenge", body: `{"success":false}`, message: "turnstile verification failed", calls: 1, level: zapcore.ErrorLevel, kind: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &turnstileOfflineTransport{body: tc.body, err: tc.transportError}
			installTurnstileOfflineTransport(t, transport)
			inject, observed := observedLoggerMiddleware(t)
			router := gin.New()
			router.Use(inject, sessions.Sessions("offline-turnstile", cookie.NewStore([]byte("offline-turnstile-test-cookie-key"))))
			var aborted, handlerCalled bool
			var verified any
			router.Use(func(c *gin.Context) {
				c.Next()
				aborted = c.IsAborted()
				verified = sessions.Default(c).Get("turnstile")
			})
			router.GET("/api/reset_password", TurnstileCheck(), func(c *gin.Context) {
				handlerCalled = true
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reset_password"+tc.query, nil))

			require.Equal(t, http.StatusOK, response.Code)
			require.True(t, aborted)
			require.False(t, handlerCalled, "rejected requests must not reach the reset-email controller")
			require.Nil(t, verified, "rejected verification must not mark the session successful")
			require.Empty(t, response.Result().Cookies())
			require.Equal(t, tc.calls, transport.calls)
			if tc.calls == 0 {
				require.JSONEq(t, `{"success":false,"message":"Turnstile token is empty"}`, response.Body.String())
			} else {
				require.Contains(t, response.Body.String(), tc.message)
				require.Contains(t, response.Body.String(), `"success":false`)
			}
			require.Len(t, observed.All(), 1)
			entry := observed.All()[0]
			require.Contains(t, entry.ContextMap()["error"], tc.message)
			require.EqualValues(t, http.StatusOK, entry.ContextMap()["status"])
			require.Equal(t, tc.level, entry.Level)
			require.Equal(t, tc.kind, entry.ContextMap()["error_kind"])
			if tc.level == zapcore.WarnLevel {
				require.Equal(t, "http handler client error", entry.Message)
				require.Equal(t, tc.message, entry.ContextMap()["error"])
				require.NotContains(t, entry.ContextMap(), "errorVerbose")
			} else {
				require.Equal(t, "http handler error", entry.Message)
			}
		})
	}
}

// TestTurnstileCheckSuccessfulPaths verifies disabled checks, existing verification and valid verification still dispatch normally.
func TestTurnstileCheckSuccessfulPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name             string
		enabled, checked bool
		query            string
		calls            int
	}{
		{name: "disabled", enabled: false},
		{name: "verified_session", enabled: true, checked: true},
		{name: "valid_verification", enabled: true, query: "?turnstile=offline-challenge", calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &turnstileOfflineTransport{body: `{"success":true}`}
			installTurnstileOfflineTransport(t, transport)
			config.TurnstileCheckEnabled = tc.enabled
			inject, observed := observedLoggerMiddleware(t)
			router := gin.New()
			router.Use(inject, sessions.Sessions("offline-turnstile", cookie.NewStore([]byte("offline-turnstile-test-cookie-key"))))
			router.Use(func(c *gin.Context) {
				if tc.checked {
					sessions.Default(c).Set("turnstile", true)
				}
				c.Next()
			})
			var handlerCalled bool
			router.GET("/api/reset_password", TurnstileCheck(), func(c *gin.Context) {
				handlerCalled = true
				require.False(t, c.IsAborted())
				if tc.enabled {
					require.Equal(t, true, sessions.Default(c).Get("turnstile"))
				}
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reset_password"+tc.query, nil))
			require.True(t, handlerCalled)
			require.Equal(t, http.StatusNoContent, response.Code)
			require.Equal(t, tc.calls, transport.calls)
			require.Empty(t, observed.All())
		})
	}
}

// TestTurnstileErrorClassificationPreservesServerLogging verifies HTTP 200 does not downgrade a classified server fault.
func TestTurnstileErrorClassificationPreservesServerLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inject, observed := observedLoggerMiddleware(t)
	router := gin.New()
	router.Use(inject)
	router.GET("/offline-server-error", func(c *gin.Context) {
		helper.RespondError(c, errkind.ServerErr(errors.New("offline server failure")))
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/offline-server-error", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"success":false,"message":"offline server failure"}`, response.Body.String())
	require.Len(t, observed.All(), 1)
	entry := observed.All()[0]
	require.Equal(t, zapcore.ErrorLevel, entry.Level)
	require.Equal(t, "http handler error", entry.Message)
	require.Equal(t, "server", entry.ContextMap()["error_kind"])
}
