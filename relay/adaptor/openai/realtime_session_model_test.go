package openai

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestEnforceRealtimeSessionUpdateBoundModel pins the session-model contract.
// Parameters: t is the test handle. Returns: none. Verified against the live
// OpenAI Realtime API on 2026-09-18: `session.created` carries `model`, clients
// echo that object back in `session.update`, upstream accepts the unchanged
// model, and upstream silently ignores a changed one. Denying every frame that
// merely mentions a model closed conformant sessions with a policy violation.
func TestEnforceRealtimeSessionUpdateBoundModel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		frame         string
		bound, origin string
		denied        bool
		wantModel     string
	}{
		{
			name:  "echoed_bound_model_forwarded",
			frame: `{"type":"session.update","session":{"type":"realtime","model":"gpt-realtime","output_modalities":["text"]}}`,
			bound: "gpt-realtime", wantModel: "gpt-realtime",
		},
		{
			name:  "user_facing_alias_rewritten_to_upstream",
			frame: `{"type":"session.update","session":{"model":"my-voice","instructions":"be brief"}}`,
			bound: "gpt-realtime-2", origin: "my-voice", wantModel: "gpt-realtime-2",
		},
		{
			name:  "different_model_denied",
			frame: `{"type":"session.update","session":{"model":"gpt-realtime-2","instructions":"x"}}`,
			bound: "gpt-realtime", origin: "gpt-realtime", denied: true,
		},
		{
			name:  "non_string_model_denied",
			frame: `{"type":"session.update","session":{"model":{"name":"gpt-realtime"}}}`,
			bound: "gpt-realtime", denied: true,
		},
		{
			name:  "null_model_forwarded",
			frame: `{"type":"session.update","session":{"model":null,"instructions":"x"}}`,
			bound: "gpt-realtime",
		},
		{
			// The legacy path could not resolve a binding; it must not start
			// rejecting sessions it previously forwarded.
			name:  "unresolved_binding_skips_enforcement",
			frame: `{"type":"session.update","session":{"model":"anything"}}`,
			bound: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := enforceRealtimeSessionUpdate([]byte(tc.frame), tc.bound, tc.origin)
			if tc.denied {
				require.Error(t, err)
				require.True(t, errors.Is(err, ErrModelSwitchDenied))
				require.Equal(t, tc.frame, string(out), "a denied frame is never rewritten")
				return
			}
			require.NoError(t, err)
			if tc.wantModel == "" {
				require.Equal(t, tc.frame, string(out), "an untouched frame is forwarded byte for byte")
				return
			}
			var decoded struct {
				Session struct {
					Model        string   `json:"model"`
					Instructions string   `json:"instructions"`
					Modalities   []string `json:"output_modalities"`
				} `json:"session"`
			}
			require.NoError(t, json.Unmarshal(out, &decoded))
			require.Equal(t, tc.wantModel, decoded.Session.Model)
		})
	}
}

// TestRealtimeWS_SessionUpdateBoundModelForwarded drives the real proxy pump.
// Parameters: t is the test handle. Returns: none. The bound model must reach
// the upstream unchanged and the session must stay open; before this, the
// documented echo pattern was answered with model_switch_denied and a close.
func TestRealtimeWS_SessionUpdateBoundModelForwarded(t *testing.T) {
	received := make(chan string, 4)
	upstream := newRecordingRealtimeUpstream(t, received)
	defer upstream.Close()

	proxy := newRealtimeProxyTestServer(t, upstream.URL)
	defer proxy.Close()

	wsURL := strings.Replace(proxy.URL, "http://", "ws://", 1) + "/v1/realtime?model=gpt-4o-realtime-preview"
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "the local realtime proxy must accept a WebSocket upgrade")
	defer clientConn.Close()

	echo := `{"type":"session.update","session":{"type":"realtime","model":"gpt-4o-realtime-preview","instructions":"be brief"}}`
	require.NoError(t, clientConn.WriteMessage(websocket.TextMessage, []byte(echo)))

	select {
	case got := <-received:
		require.JSONEq(t, echo, got, "the bound model is forwarded unchanged")
	case <-time.After(3 * time.Second):
		t.Fatal("upstream never received the session.update carrying the bound model")
	}

	// The session must still be usable: no error event, no policy close.
	require.NoError(t, clientConn.SetReadDeadline(time.Now().Add(500*time.Millisecond)))
	_, msg, readErr := clientConn.ReadMessage()
	if readErr == nil {
		var event map[string]any
		require.NoError(t, json.Unmarshal(msg, &event))
		require.NotEqual(t, "error", event["type"], "a conformant session.update must not raise an error event")
		return
	}
	var closeErr *websocket.CloseError
	require.False(t, errors.As(readErr, &closeErr), "a conformant session.update must not close the session")
}
