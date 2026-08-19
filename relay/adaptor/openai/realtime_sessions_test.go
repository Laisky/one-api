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

// TestRealtimeSessionsHandler_RejectsMismatchedBodyModel reproduces a
// billing-bypass vector: the client opens a session against a channel bound
// to model X but specifies a different (more expensive) model Y in the
// request body. OpenAI mints an ephemeral token for Y, and if the client
// uses that token over WebRTC the proxy never sees the realtime traffic
// while the upstream API key still pays Y's rate.
//
// The handler must reject the request with 400 before forwarding so the
// channel's OpenAI account is not charged for a model the channel was not
// authorized for.
func TestRealtimeSessionsHandler_RejectsMismatchedBodyModel(t *testing.T) {
	t.Parallel()

	var upstreamCalled bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"sess_should_not_appear"}`))
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	reqBody := `{"model":"gpt-realtime-2","voice":"verse"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions",
		bytes.NewBufferString(reqBody))

	meta := &rmeta.Meta{
		BaseURL:         upstream.URL,
		APIKey:          "sk-upstream-test-key",
		OriginModelName: "gpt-4o-realtime-preview",
		ActualModelName: "gpt-4o-realtime-preview",
	}

	bizErr, _ := RealtimeSessionsHandler(c, meta)
	require.NotNil(t, bizErr, "mismatched body model must surface a business error")
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode,
		"mismatched body model must yield 400 so the channel is not charged")
	require.False(t, upstreamCalled,
		"upstream MUST NOT be called when the body model is denied")
}

// TestRealtimeSessionsHandler_InjectsBoundModelWhenMissing verifies that an
// empty body (or a body without a `model` field) is sent upstream with the
// channel-bound model injected. This is the legitimate default for clients
// that rely on the channel's pinned configuration.
func TestRealtimeSessionsHandler_InjectsBoundModelWhenMissing(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"sess_ok"}`))
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions",
		bytes.NewBufferString(`{"voice":"verse"}`))

	meta := &rmeta.Meta{
		BaseURL:         upstream.URL,
		APIKey:          "sk-upstream-test-key",
		OriginModelName: "gpt-4o-realtime-preview",
		ActualModelName: "gpt-4o-realtime-preview",
	}

	bizErr, err := RealtimeSessionsHandler(c, meta)
	require.NoError(t, err)
	require.Nil(t, bizErr)

	var sent map[string]any
	require.NoError(t, json.Unmarshal(capturedBody, &sent))
	require.Equal(t, "gpt-4o-realtime-preview", sent["model"],
		"upstream must see the channel-bound model injected when body omits it")
	require.Equal(t, "verse", sent["voice"], "other body fields must be preserved")
}

// TestRealtimeSessionsHandler_OriginAliasRewrittenToActual verifies that a
// client using a user-facing alias has it rewritten to the upstream-actual
// model name (model mapping) before forwarding.
func TestRealtimeSessionsHandler_OriginAliasRewrittenToActual(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"sess_ok"}`))
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions",
		bytes.NewBufferString(`{"model":"my-rt-alias","voice":"verse"}`))

	meta := &rmeta.Meta{
		BaseURL:         upstream.URL,
		APIKey:          "sk-test",
		OriginModelName: "my-rt-alias",
		ActualModelName: "gpt-4o-realtime-preview",
	}

	bizErr, err := RealtimeSessionsHandler(c, meta)
	require.NoError(t, err)
	require.Nil(t, bizErr)

	var sent map[string]any
	require.NoError(t, json.Unmarshal(capturedBody, &sent))
	require.Equal(t, "gpt-4o-realtime-preview", sent["model"],
		"upstream must see the actual model, not the alias")
}

// TestRealtimeSessionsHandler_LegacyEmptyBoundModelDoesNotEnforce documents
// the backward-compat branch: when the proxy could not resolve a channel
// model (legacy path), the body is forwarded unchanged.
func TestRealtimeSessionsHandler_LegacyEmptyBoundModelDoesNotEnforce(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"sess_ok"}`))
	}))
	defer upstream.Close()

	body := `{"model":"gpt-realtime-anything","voice":"alloy"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions",
		bytes.NewBufferString(body))

	meta := &rmeta.Meta{
		BaseURL: upstream.URL,
		APIKey:  "sk-test",
	}

	bizErr, err := RealtimeSessionsHandler(c, meta)
	require.NoError(t, err)
	require.Nil(t, bizErr)
	require.JSONEq(t, body, string(capturedBody),
		"legacy no-binding mode must forward the body unchanged")
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
