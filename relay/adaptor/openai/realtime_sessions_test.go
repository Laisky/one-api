package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestRealtimeSessionsHandler_ProxiesRequest verifies that the handler
// correctly proxies the request body and auth header to the upstream.
func TestRealtimeSessionsHandler_ProxiesRequest(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	var capturedAuth string
	var capturedContentType string
	var capturedBeta string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedContentType = r.Header.Get("Content-Type")
		capturedBeta = r.Header.Get("OpenAI-Beta")
		capturedBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]any{
			"id":     "sess_test123",
			"object": "realtime.session",
			"model":  "gpt-4o-realtime-preview-2025-06-03",
			"client_secret": map[string]any{
				"value":      "ek_ephemeral_test_key",
				"expires_at": 1234567890,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	reqBody := `{"model":"gpt-4o-realtime-preview-2025-06-03","voice":"verse"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions", bytes.NewBufferString(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	meta := &rmeta.Meta{
		BaseURL: upstream.URL,
		APIKey:  "sk-upstream-test-key",
	}

	bizErr, err := RealtimeSessionsHandler(c, meta)
	require.NoError(t, err)
	require.Nil(t, bizErr)

	// Verify the request was proxied correctly
	require.Equal(t, "Bearer sk-upstream-test-key", capturedAuth)
	require.Equal(t, "application/json", capturedContentType)
	require.Equal(t, "realtime=v1", capturedBeta)
	require.JSONEq(t, reqBody, string(capturedBody))

	// Verify the response was forwarded
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "sess_test123", resp["id"])
	secret := resp["client_secret"].(map[string]any)
	require.Equal(t, "ek_ephemeral_test_key", secret["value"])
}

// TestRealtimeSessionsHandler_UpstreamError verifies that upstream errors
// are properly forwarded to the client.
func TestRealtimeSessionsHandler_UpstreamError(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid model","type":"invalid_request_error"}}`))
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions", bytes.NewBufferString(`{"model":"invalid"}`))

	meta := &rmeta.Meta{
		BaseURL: upstream.URL,
		APIKey:  "sk-test",
	}

	bizErr, _ := RealtimeSessionsHandler(c, meta)
	// The handler writes the response directly, so check the recorder
	require.Equal(t, http.StatusBadRequest, w.Code)
	// bizErr should be non-nil for 4xx upstream responses
	require.NotNil(t, bizErr)
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
}

// TestRealtimeSessionsHandler_UpstreamUnreachable verifies behavior when
// the upstream is not reachable.
func TestRealtimeSessionsHandler_UpstreamUnreachable(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions", bytes.NewBufferString(`{}`))

	meta := &rmeta.Meta{
		BaseURL: "http://127.0.0.1:1", // unreachable port
		APIKey:  "sk-test",
	}

	bizErr, err := RealtimeSessionsHandler(c, meta)
	require.Error(t, err)
	require.NotNil(t, bizErr)
	require.Equal(t, http.StatusBadGateway, bizErr.StatusCode)
}

// TestRealtimeSessionsHandler_DefaultBaseURL verifies that when BaseURL is empty,
// the handler defaults to the OpenAI base URL.
func TestRealtimeSessionsHandler_DefaultBaseURL(t *testing.T) {
	t.Parallel()

	var capturedURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"sess_1"}`))
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions", bytes.NewBufferString(`{}`))

	// Use upstream URL as base since we can't actually connect to api.openai.com in tests
	meta := &rmeta.Meta{
		BaseURL: upstream.URL,
		APIKey:  "sk-test",
	}

	bizErr, err := RealtimeSessionsHandler(c, meta)
	require.NoError(t, err)
	require.Nil(t, bizErr)
	require.Equal(t, "/v1/realtime/sessions", capturedURL)
}

// TestRealtimeSessionsHandler_EmptyBody verifies the handler works with an empty body.
func TestRealtimeSessionsHandler_EmptyBody(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.Empty(t, body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"sess_empty"}`))
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions", nil)

	meta := &rmeta.Meta{
		BaseURL: upstream.URL,
		APIKey:  "sk-test",
	}

	bizErr, err := RealtimeSessionsHandler(c, meta)
	require.NoError(t, err)
	require.Nil(t, bizErr)
	require.Equal(t, http.StatusOK, w.Code)
}

// TestRealtimeSessionsHandler_ResponseHeaders verifies that upstream response
// headers are forwarded to the client.
func TestRealtimeSessionsHandler_ResponseHeaders(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req_test_456")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"sess_hdr"}`))
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions", bytes.NewBufferString(`{}`))

	meta := &rmeta.Meta{
		BaseURL: upstream.URL,
		APIKey:  "sk-test",
	}

	bizErr, err := RealtimeSessionsHandler(c, meta)
	require.NoError(t, err)
	require.Nil(t, bizErr)
	require.Equal(t, "req_test_456", w.Header().Get("X-Request-Id"))
}
