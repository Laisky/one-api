package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/controller"
)

// TestReviewRealtimeRoutesRejectCredentialMinting exercises the actual relay
// router and production not-found handler; no fake authenticated identity is used.
func TestReviewRealtimeRoutesRejectCredentialMinting(t *testing.T) {
	r := gin.New()
	SetRelayRouter(r)
	r.NoRoute(controller.RelayNotFound)
	for _, path := range []string{
		"/v1/realtime/sessions", "/v1/realtime/sessions/",
		"/v1/realtime/transcription_sessions", "/v1/realtime/client_secrets",
		"/v1/realtime/calls", "/v1/realtime/%73essions",
	} {
		for _, auth := range []string{"", "Bearer invalid-fixture-token"} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-realtime"}`))
			req.Header.Set("Authorization", auth)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code, path)
			require.Contains(t, w.Header().Get("Content-Type"), "application/json")
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
			require.Contains(t, payload, "error")
			require.NotContains(t, payload, "client_secret")
		}
	}
	found := false
	for _, route := range r.Routes() {
		if route.Method == http.MethodGet && route.Path == "/v1/realtime" {
			found = true
		}
		require.NotEqual(t, "/v1/realtime/sessions", route.Path)
	}
	require.True(t, found, "the metered WebSocket endpoint must remain available")
}
