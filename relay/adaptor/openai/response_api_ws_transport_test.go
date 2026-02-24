package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

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

// TestAdaptorDoRequest_ResponseAPIWebSocketNonStream verifies that OpenAI Response API
// requests with stream=false are executed via websocket and materialized back into a
// single JSON response payload.
//
// Parameters:
//   - t: Go testing handle.
func TestAdaptorDoRequest_ResponseAPIWebSocketNonStream(t *testing.T) {
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

		completed := map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":     "resp_non_stream_1",
				"object": "response",
				"status": "completed",
				"model":  "gpt-5.2",
				"output": []map[string]any{
					{
						"type":    "message",
						"role":    "assistant",
						"content": []map[string]any{{"type": "output_text", "text": "ok"}},
					},
				},
				"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
			},
		}
		payload, _ := json.Marshal(completed)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, payload))
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
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	var finalResp ResponseAPIResponse
	require.NoError(t, json.Unmarshal(body, &finalResp))
	require.Equal(t, "resp_non_stream_1", finalResp.Id)
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
