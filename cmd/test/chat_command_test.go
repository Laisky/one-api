package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestParseChatResponseArgs verifies chat response CLI flag parsing and defaults.
func TestParseChatResponseArgs(t *testing.T) {
	t.Parallel()

	cfg := config{
		APIBase: "https://example.test",
		Token:   "sk-test",
		Models:  []string{"gpt-5-mini"},
	}

	opts, err := parseChatResponseArgs([]string{"--ws", "--prompt", "hello"}, cfg)
	require.NoError(t, err)
	require.True(t, opts.useWebSocket)
	require.Equal(t, "https://example.test/v1/responses", opts.apiEndpoint)
	require.Equal(t, "sk-test", opts.apiToken)
	require.Equal(t, "gpt-5-mini", opts.model)
	require.Equal(t, "hello", opts.prompt)
	require.Equal(t, defaultChatResponseTimeout, opts.timeout)
}

// TestParseChatResponseArgsValidation verifies required argument checks.
func TestParseChatResponseArgsValidation(t *testing.T) {
	t.Parallel()

	cfg := config{APIBase: "https://example.test", Token: "", Models: nil}
	_, err := parseChatResponseArgs([]string{"--ws"}, cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "token is required")
}

// TestResolveChatResponseWSEndpoint verifies scheme and path normalization.
func TestResolveChatResponseWSEndpoint(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "https endpoint", input: "https://oneapi.test/v1/responses", expected: "wss://oneapi.test/v1/responses"},
		{name: "http endpoint", input: "http://oneapi.test/v1/responses", expected: "ws://oneapi.test/v1/responses"},
		{name: "empty path", input: "https://oneapi.test", expected: "wss://oneapi.test/v1/responses"},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, err := resolveChatResponseWSEndpoint(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}

// TestResolveChatResponseWSEndpointInvalid verifies invalid endpoints are rejected.
func TestResolveChatResponseWSEndpointInvalid(t *testing.T) {
	t.Parallel()

	_, err := resolveChatResponseWSEndpoint("ftp://oneapi.test/v1/responses")
	require.Error(t, err)
}

// TestIsResponseWebSocketTerminalMessage verifies terminal event detection.
func TestIsResponseWebSocketTerminalMessage(t *testing.T) {
	t.Parallel()

	require.True(t, isResponseWebSocketTerminalMessage([]byte(`{"type":"response.completed"}`)))
	require.True(t, isResponseWebSocketTerminalMessage([]byte(`{"type":"error","error":{"message":"bad"}}`)))
	require.False(t, isResponseWebSocketTerminalMessage([]byte(`{"type":"response.output_text.delta","delta":"hi"}`)))
}

// TestResponseWebSocketMessageError verifies websocket error payload decoding.
func TestResponseWebSocketMessageError(t *testing.T) {
	t.Parallel()

	err := responseWebSocketMessageError([]byte(`{"type":"error","status":400,"error":{"code":"previous_response_not_found","message":"not found"}}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "previous_response_not_found")

	noErr := responseWebSocketMessageError([]byte(`{"type":"response.completed"}`))
	require.NoError(t, noErr)
}

// TestParseChatResponseArgsCustomTimeout verifies duration parsing for timeout option.
func TestParseChatResponseArgsCustomTimeout(t *testing.T) {
	t.Parallel()

	cfg := config{APIBase: "https://example.test", Token: "sk", Models: []string{"gpt-5-mini"}}
	opts, err := parseChatResponseArgs([]string{"--ws", "--timeout", "15s"}, cfg)
	require.NoError(t, err)
	require.Equal(t, 15*time.Second, opts.timeout)
}

// TestRunChatResponseWebSocketProbeNormalClose verifies probe success when server
// closes websocket normally after receiving the request event.
func TestRunChatResponseWebSocketProbeNormalClose(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()

		_, _, err = conn.ReadMessage()
		require.NoError(t, err)

		require.NoError(t, conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
			time.Now().Add(time.Second),
		))
	}))
	defer server.Close()

	logger, err := glog.NewConsoleWithName("oneapi-test", glog.LevelInfo)
	require.NoError(t, err)

	opts := chatResponseOptions{
		apiEndpoint: server.URL + "/v1/responses",
		apiToken:    "sk-test",
		model:       "gpt-5-mini",
		prompt:      defaultChatResponsePrompt,
		timeout:     5 * time.Second,
	}

	err = runChatResponseWebSocketProbe(context.Background(), logger, opts)
	require.NoError(t, err)
}
