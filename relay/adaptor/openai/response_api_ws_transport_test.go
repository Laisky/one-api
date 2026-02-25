package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	dbmodel "github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// TestAdaptorDoRequest_ResponseAPIWebSocketStream verifies that OpenAI Response API
// requests with stream=true are sent via websocket and bridged back as SSE.
//
// Parameters:
//   - t: Go testing handle.
func TestAdaptorDoRequest_ResponseAPIWebSocketStream(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		_, payload, err := conn.ReadMessage()
		require.NoError(t, err)

		var event map[string]any
		require.NoError(t, json.Unmarshal(payload, &event))
		require.Equal(t, "response.create", event["type"])
		_, hasStream := event["stream"]
		require.False(t, hasStream)
		_, hasBackground := event["background"]
		require.False(t, hasBackground)

		delta := map[string]any{
			"type":  "response.output_text.delta",
			"delta": "hello",
		}
		completed := map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":     "resp_stream_1",
				"object": "response",
				"status": "completed",
				"model":  "gpt-5.2",
				"output": []map[string]any{
					{
						"type": "message",
						"role": "assistant",
						"content": []map[string]any{
							{"type": "output_text", "text": "hello"},
						},
					},
				},
				"usage": map[string]any{
					"input_tokens":  3,
					"output_tokens": 2,
					"total_tokens":  5,
				},
			},
		}
		deltaPayload, _ := json.Marshal(delta)
		completedPayload, _ := json.Marshal(completed)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, deltaPayload))
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, completedPayload))
	}))
	defer server.Close()

	requestPayload := []byte(`{"model":"gpt-5.2","stream":true,"background":false,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ping"}]}]}`)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	metaInfo := &meta.Meta{
		Mode:            relaymode.ResponseAPI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         server.URL,
		APIKey:          "test-key",
		RequestURLPath:  "/v1/responses",
		ActualModelName: "gpt-5.2",
	}

	a := &Adaptor{}
	a.Init(metaInfo)
	resp, err := a.DoRequest(ctx, metaInfo, bytes.NewReader(requestPayload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, string(body), "response.output_text.delta")
	require.Contains(t, string(body), "response.completed")
	require.Contains(t, string(body), "data: [DONE]")
}

// TestAdaptorDoRequest_ResponseAPINonStreamUsesHTTP verifies that OpenAI Response API
// requests with stream=false always use HTTP transport and skip websocket upgrades.
//
// Parameters:
//   - t: Go testing handle.
func TestAdaptorDoRequest_ResponseAPINonStreamUsesHTTP(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalDB := dbmodel.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	dbmodel.DB = db
	t.Cleanup(func() {
		dbmodel.DB = originalDB
	})

	var websocketAttempted atomic.Bool
	var httpHandled atomic.Bool
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if websocket.IsWebSocketUpgrade(r) {
			websocketAttempted.Store(true)
			_, err = upgrader.Upgrade(w, r, nil)
			require.NoError(t, err)
			return
		}

		httpHandled.Store(true)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"stream":false`)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_non_stream_http_1","object":"response","status":"completed","model":"gpt-5.2","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	requestPayload := []byte(`{"model":"gpt-5.2","stream":false,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ping"}]}]}`)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	metaInfo := &meta.Meta{
		Mode:            relaymode.ResponseAPI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         server.URL,
		APIKey:          "test-key",
		RequestURLPath:  "/v1/responses",
		ActualModelName: "gpt-5.2",
	}

	a := &Adaptor{}
	a.Init(metaInfo)
	resp, err := a.DoRequest(ctx, metaInfo, bytes.NewReader(requestPayload))
	require.NoError(t, err)
	require.False(t, websocketAttempted.Load())
	require.True(t, httpHandled.Load())
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	var finalResp ResponseAPIResponse
	require.NoError(t, json.Unmarshal(body, &finalResp))
	require.Equal(t, "resp_non_stream_http_1", finalResp.Id)
	require.Equal(t, "response", finalResp.Object)
	require.Equal(t, "completed", finalResp.Status)
}

// TestAdaptorDoRequest_ResponseAPIWebSocketFallbackForBackground verifies background
// requests keep HTTP transport semantics and do not switch to websocket.
//
// Parameters:
//   - t: Go testing handle.
func TestDoResponseAPIRequestViaWebSocket_FallbackForBackground(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	requestPayload := []byte(`{"model":"gpt-5.2","stream":false,"background":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ping"}]}]}`)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	metaInfo := &meta.Meta{
		Mode:            relaymode.ResponseAPI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         "https://api.openai.com",
		APIKey:          "test-key",
		RequestURLPath:  "/v1/responses",
		ActualModelName: "gpt-5.2",
	}

	a := &Adaptor{}
	a.Init(metaInfo)
	resp, handled, err := doResponseAPIRequestViaWebSocket(ctx, a, metaInfo, bytes.NewReader(requestPayload))
	require.NoError(t, err)
	require.False(t, handled)
	require.Nil(t, resp)
}

// TestAdaptorDoRequest_ResponseAPIWebSocketNormalCloseFallback verifies that when the
// upstream websocket closes normally before emitting any event, adaptor falls back to
// HTTP transport and preserves the request payload.
//
// Parameters:
//   - t: Go testing handle.
func TestAdaptorDoRequest_ResponseAPIWebSocketNormalCloseFallback(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalDB := dbmodel.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	dbmodel.DB = db
	t.Cleanup(func() {
		dbmodel.DB = originalDB
	})

	var websocketAttempted atomic.Bool
	var httpFallbackCalled atomic.Bool

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if websocket.IsWebSocketUpgrade(r) {
			websocketAttempted.Store(true)
			conn, err := upgrader.Upgrade(w, r, nil)
			require.NoError(t, err)
			defer conn.Close()

			_, _, err = conn.ReadMessage()
			require.NoError(t, err)
			require.NoError(t, conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "upstream normal close"), time.Now().Add(time.Second)))
			return
		}

		httpFallbackCalled.Store(true)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "\"model\":\"gpt-5.2\"")
		require.Contains(t, string(body), "\"input\"")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_http_fallback_1","object":"response","status":"completed","model":"gpt-5.2","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	requestPayload := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ping"}]}]}`)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	metaInfo := &meta.Meta{
		Mode:            relaymode.ResponseAPI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         server.URL,
		APIKey:          "test-key",
		RequestURLPath:  "/v1/responses",
		ActualModelName: "gpt-5.2",
	}

	a := &Adaptor{}
	a.Init(metaInfo)
	resp, err := a.DoRequest(ctx, metaInfo, bytes.NewReader(requestPayload))
	require.NoError(t, err)
	require.True(t, websocketAttempted.Load())
	require.True(t, httpFallbackCalled.Load())
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	var finalResp ResponseAPIResponse
	require.NoError(t, json.Unmarshal(body, &finalResp))
	require.Equal(t, "resp_http_fallback_1", finalResp.Id)
	require.Equal(t, "completed", finalResp.Status)
}

// TestAdaptorDoRequest_ResponseAPIWebSocketBackgroundFallbackHTTPBody verifies that
// background=true keeps HTTP transport and still forwards a non-empty request body.
//
// Parameters:
//   - t: Go testing handle.
func TestAdaptorDoRequest_ResponseAPIWebSocketBackgroundFallbackHTTPBody(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalDB := dbmodel.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	dbmodel.DB = db
	t.Cleanup(func() {
		dbmodel.DB = originalDB
	})

	var httpCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		httpCalls.Add(1)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NotEmpty(t, body)
		require.Contains(t, string(body), "\"background\":true")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_background_http_1","object":"response","status":"completed","model":"gpt-5.2","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	requestPayload := []byte(`{"model":"gpt-5.2","stream":false,"background":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ping"}]}]}`)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	metaInfo := &meta.Meta{
		Mode:            relaymode.ResponseAPI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         server.URL,
		APIKey:          "test-key",
		RequestURLPath:  "/v1/responses",
		ActualModelName: "gpt-5.2",
	}

	a := &Adaptor{}
	a.Init(metaInfo)
	resp, err := a.DoRequest(ctx, metaInfo, bytes.NewReader(requestPayload))
	require.NoError(t, err)
	require.Equal(t, int32(1), httpCalls.Load())
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	var finalResp ResponseAPIResponse
	require.NoError(t, json.Unmarshal(body, &finalResp))
	require.Equal(t, "resp_background_http_1", finalResp.Id)
	require.Equal(t, "completed", finalResp.Status)
}

// TestAdaptorDoRequest_ResponseAPIWebSocketConnectionLimitErrorFallback verifies
// websocket transport falls back to HTTP when upstream asks the client to create
// a new websocket connection.
//
// Parameters:
//   - t: Go testing handle.
func TestAdaptorDoRequest_ResponseAPIWebSocketConnectionLimitErrorFallback(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalDB := dbmodel.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	dbmodel.DB = db
	t.Cleanup(func() {
		dbmodel.DB = originalDB
	})

	var websocketAttempted atomic.Bool
	var httpFallbackCalled atomic.Bool

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if websocket.IsWebSocketUpgrade(r) {
			websocketAttempted.Store(true)
			conn, err := upgrader.Upgrade(w, r, nil)
			require.NoError(t, err)
			defer conn.Close()

			_, _, err = conn.ReadMessage()
			require.NoError(t, err)

			errorPayload := map[string]any{
				"type":   "error",
				"status": http.StatusBadRequest,
				"error": map[string]any{
					"type":    "invalid_request_error",
					"code":    "websocket_connection_limit_reached",
					"message": "Responses websocket connection limit reached (60 minutes). Create a new websocket connection to continue.",
				},
			}
			payload, err := json.Marshal(errorPayload)
			require.NoError(t, err)
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, payload))
			return
		}

		httpFallbackCalled.Store(true)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_http_fallback_ws_limit","object":"response","status":"completed","model":"gpt-5.2","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	requestPayload := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ping"}]}]}`)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	metaInfo := &meta.Meta{
		Mode:            relaymode.ResponseAPI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         server.URL,
		APIKey:          "test-key",
		RequestURLPath:  "/v1/responses",
		ActualModelName: "gpt-5.2",
	}

	a := &Adaptor{}
	a.Init(metaInfo)
	resp, err := a.DoRequest(ctx, metaInfo, bytes.NewReader(requestPayload))
	require.NoError(t, err)
	require.True(t, websocketAttempted.Load())
	require.True(t, httpFallbackCalled.Load())
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	var finalResp ResponseAPIResponse
	require.NoError(t, json.Unmarshal(body, &finalResp))
	require.Equal(t, "resp_http_fallback_ws_limit", finalResp.Id)
	require.Equal(t, "completed", finalResp.Status)
}

// TestDoResponseAPIRequestViaWebSocket_ErrorEventPassThrough verifies non-reconnect
// websocket error events still return synthesized error responses for compatibility.
//
// Parameters:
//   - t: Go testing handle.
func TestDoResponseAPIRequestViaWebSocket_ErrorEventPassThrough(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		_, _, err = conn.ReadMessage()
		require.NoError(t, err)

		errorPayload := map[string]any{
			"type":   "error",
			"status": http.StatusBadRequest,
			"error": map[string]any{
				"type":    "invalid_request_error",
				"code":    "some_other_error",
				"message": "some client-side validation issue",
			},
		}
		payload, err := json.Marshal(errorPayload)
		require.NoError(t, err)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, payload))
	}))
	defer server.Close()

	requestPayload := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ping"}]}]}`)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	metaInfo := &meta.Meta{
		Mode:            relaymode.ResponseAPI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         server.URL,
		APIKey:          "test-key",
		RequestURLPath:  "/v1/responses",
		ActualModelName: "gpt-5.2",
	}

	a := &Adaptor{}
	a.Init(metaInfo)

	resp, handled, err := doResponseAPIRequestViaWebSocket(ctx, a, metaInfo, bytes.NewReader(requestPayload))
	require.NoError(t, err)
	require.True(t, handled)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, string(body), "some_other_error")
}

// TestShouldFallbackToHTTPForWebSocketError_MessageOnly verifies fallback detection
// works when upstream omits error.code but provides reconnection instructions.
//
// Parameters:
//   - t: Go testing handle.
func TestShouldFallbackToHTTPForWebSocketError_MessageOnly(t *testing.T) {
	t.Helper()

	payload := []byte(`{"type":"error","status":400,"error":{"type":"invalid_request_error","message":"Responses websocket connection limit reached (60 minutes). Create a new websocket connection to continue."}}`)
	require.True(t, shouldFallbackToHTTPForWebSocketError(payload))

	errResp, ok := tryBuildWebSocketErrorResponse(payload)
	require.True(t, ok)
	require.NotNil(t, errResp)
	require.Equal(t, http.StatusBadRequest, errResp.StatusCode)
}

// TestParseWebSocketErrorPayload_ResponseFailed verifies nested error payloads from
// response.failed events are converted to synthesized HTTP error responses.
//
// Parameters:
//   - t: Go testing handle.
func TestParseWebSocketErrorPayload_ResponseFailed(t *testing.T) {
	t.Helper()

	payload := []byte(`{"type":"response.failed","response":{"id":"resp_123","status":"failed","error":{"type":"invalid_request_error","code":"websocket_connection_limit_reached","message":"Responses websocket connection limit reached (60 minutes). Create a new websocket connection to continue."}}}`)

	require.Equal(t, wsErrorCodeConnectionLimitReached, readWebSocketErrorCode(payload))
	require.True(t, shouldFallbackToHTTPForWebSocketError(payload))

	errResp, ok := tryBuildWebSocketErrorResponse(payload)
	require.True(t, ok)
	require.NotNil(t, errResp)
	require.Equal(t, http.StatusBadRequest, errResp.StatusCode)

	body, err := io.ReadAll(errResp.Body)
	require.NoError(t, err)
	require.NoError(t, errResp.Body.Close())
	require.Contains(t, string(body), wsErrorCodeConnectionLimitReached)
}

// TestShouldFallbackToHTTPForWebSocketClose verifies transient websocket close
// errors are classified for HTTP fallback while terminal protocol errors are not.
//
// Parameters:
//   - t: Go testing handle.
