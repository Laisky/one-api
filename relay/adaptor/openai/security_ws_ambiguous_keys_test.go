package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
)

// TestSecurityResponseWSAmbiguousGuardedKeys proves a Responses WebSocket frame
// cannot name a guarded key (type, model, previous_response_id, conversation,
// background) twice or under a case-folded spelling. The guards decode a map
// that keeps only the last exact key, while a provider may read the first
// duplicate or fold case, so an ambiguous frame would forward a value the
// gateway never checked. Every such frame must close the socket before dispatch.
func TestSecurityResponseWSAmbiguousGuardedKeys(t *testing.T) {
	cases := map[string]string{
		"model-exact-duplicate":         `{"type":"response.create","model":"gpt-5","model":"gpt-4o-mini","input":"hi"}`,
		"model-case-folded":             `{"type":"response.create","model":"gpt-4o-mini","Model":"gpt-5","input":"hi"}`,
		"model-escaped-duplicate":       `{"type":"response.create","model":"gpt-5","model":"gpt-4o-mini","input":"hi"}`,
		"type-duplicate":                `{"type":"response.cancel","type":"response.create","model":"gpt-4o-mini","input":"hi"}`,
		"conversation-duplicate":        `{"type":"response.create","model":"gpt-4o-mini","conversation":"conv_x","conversation":null,"input":"hi"}`,
		"previous-response-case-folded": `{"type":"response.create","model":"gpt-4o-mini","previous_response_id":"resp_gateway","Previous_Response_Id":"resp_foreign_provider","input":"hi"}`,
		"background-duplicate":          `{"type":"response.create","model":"gpt-4o-mini","background":false,"Background":false,"input":"hi"}`,
	}
	for name, frame := range cases {
		t.Run(name, func(t *testing.T) {
			store := state.NewMemoryStore(state.DefaultLimits())
			state.SetForTest(store)
			t.Cleanup(func() { state.SetForTest(nil) })
			_, err := store.CreateResponse(context.Background(), &state.ResponseStateRecord{GatewayResponseID: "resp_gateway", Owner: state.OwnerScope{UserID: 1, TokenID: 2}, StoreMode: true, Status: state.StatusCompleted, Binding: &state.ProviderBinding{ChannelID: 3, APIType: 0, UpstreamResponseID: "resp_bound_provider"}}, "")
			require.NoError(t, err)

			outcome := runResponseWSFrame(t, frame)
			require.Empty(t, outcome.forwarded, "an ambiguous frame must never reach the provider socket")
			require.Error(t, outcome.readErr, "an ambiguous frame must close the socket")
			require.NotNil(t, outcome.bizErr, "rejection before execution must release the handshake reservation")
		})
	}

	t.Run("canonical-control", func(t *testing.T) {
		state.SetForTest(nil)
		outcome := runResponseWSFrame(t, `{"type":"response.create","model":"gpt-4o-mini","input":"hi"}`)
		require.NoError(t, outcome.readErr)
		require.Nil(t, outcome.bizErr)
		require.Len(t, outcome.forwarded, 1)
	})
}

// TestSecurityResponseWSSessionLocalDuplicatePrevious proves a duplicated
// previous_response_id cannot pair a provider-confirmed same-socket ID (which is
// forwarded without re-encoding) with an unowned provider ID.
func TestSecurityResponseWSSessionLocalDuplicatePrevious(t *testing.T) {
	state.SetForTest(nil)
	ownership := &responseWSSessionOwnership{}
	ownership.observe("resp_session")
	meta := &rmeta.Meta{UserId: 1, TokenId: 2, ChannelId: 3}

	out, err := ownership.authorize(context.Background(), meta, []byte(`{"type":"response.create","previous_response_id":"resp_foreign_provider","previous_response_id":"resp_session"}`))
	require.Error(t, err, "an ambiguous previous_response_id must be rejected; forwarded=%s", out)
	require.Nil(t, out)

	out, err = ownership.authorize(context.Background(), meta, []byte(`{"type":"response.create","previous_response_id":"resp_session"}`))
	require.NoError(t, err, "a single same-socket continuation stays usable")
	require.Contains(t, string(out), "resp_session")
}

// TestSecurityWSModelGuardsRejectAmbiguousKeys proves the standalone model
// guards reject duplicate and case-folded model selectors instead of returning
// the raw bytes after validating only the last decoded copy.
func TestSecurityWSModelGuardsRejectAmbiguousKeys(t *testing.T) {
	for name, frame := range map[string]string{
		"exact-duplicate": `{"type":"response.create","model":"gpt-5","model":"gpt-4o-mini"}`,
		"case-folded":     `{"type":"response.create","model":"gpt-4o-mini","MODEL":"gpt-5"}`,
		"type-duplicate":  `{"type":"response.cancel","type":"response.create","model":"gpt-4o-mini"}`,
	} {
		t.Run("response-create/"+name, func(t *testing.T) {
			out, err := enforceResponseCreateModel([]byte(frame), "gpt-4o-mini", "gpt-4o-mini")
			require.ErrorIs(t, err, ErrModelSwitchDenied, "forwarded=%s", out)
		})
	}

	meta := &rmeta.Meta{OriginModelName: "gpt-4o-realtime-preview", ActualModelName: "gpt-4o-realtime-preview"}
	for name, body := range map[string]string{
		"exact-duplicate": `{"model":"gpt-realtime-2","model":"gpt-4o-realtime-preview"}`,
		"case-folded":     `{"model":"gpt-4o-realtime-preview","Model":"gpt-realtime-2"}`,
	} {
		t.Run("realtime-sessions/"+name, func(t *testing.T) {
			out, bizErr := enforceRealtimeSessionsBodyModel([]byte(body), meta)
			require.NotNil(t, bizErr, "forwarded=%s", out)
			require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
			require.Nil(t, out)
		})
	}

	for name, frame := range map[string]string{
		"type-duplicate":         `{"type":"session.update","type":"conversation.item.create","session":{"model":"gpt-realtime-2"}}`,
		"session-duplicate":      `{"type":"session.update","session":{"model":"gpt-realtime-2"},"session":{"instructions":"x"}}`,
		"session-case-folded":    `{"type":"session.update","Session":{"model":"gpt-realtime-2"}}`,
		"nested-model-duplicate": `{"type":"session.update","session":{"model":"gpt-realtime-2","model":"gpt-4o-realtime-preview"}}`,
		"nested-model-folded":    `{"type":"session.update","session":{"model":"gpt-4o-realtime-preview","Model":"gpt-realtime-2"}}`,
		"session-type-duplicate": `{"type":"session.update","session":{"type":"transcription","type":"realtime"}}`,
	} {
		t.Run("realtime-update/"+name, func(t *testing.T) {
			out, err := enforceRealtimeSessionUpdate([]byte(frame), "gpt-4o-realtime-preview", "gpt-4o-realtime-preview", false)
			require.ErrorIs(t, err, ErrModelSwitchDenied, "forwarded=%s", out)
		})
	}
	for name, frame := range map[string]string{
		"nested-transcription-duplicate": `{"type":"transcription_session.update","session":{"audio":{"input":{"transcription":{"model":"gpt-4o-transcribe","model":"gpt-4o-mini-transcribe"}}}}}`,
		"root-transcription-duplicate":   `{"type":"transcription_session.update","input_audio_transcription":{"model":"gpt-4o-transcribe"},"input_audio_transcription":{"language":"en"}}`,
	} {
		t.Run("realtime-transcription/"+name, func(t *testing.T) {
			out, err := enforceRealtimeSessionUpdate([]byte(frame), "gpt-4o-mini-transcribe", "gpt-4o-mini-transcribe", true)
			require.ErrorIs(t, err, ErrModelSwitchDenied, "forwarded=%s", out)
		})
	}

	t.Run("realtime-controls", func(t *testing.T) {
		for _, frame := range []string{
			`{"type":"session.update","session":{"instructions":"x"}}`,
			`{"type":"session.update","session":{"type":"realtime","model":"gpt-4o-realtime-preview"}}`,
			`{"type":"input_audio_buffer.append","audio":"AAAA"}`,
		} {
			out, err := enforceRealtimeSessionUpdate([]byte(frame), "gpt-4o-realtime-preview", "gpt-4o-realtime-preview", false)
			require.NoError(t, err)
			require.JSONEq(t, frame, string(out))
		}
	})
}

// TestSecurityRealtimeWSAmbiguousKeysNeverForwarded drives the real realtime
// pump: an ambiguous session.update must raise model_switch_denied and close the
// socket without the provider ever receiving the frame.
func TestSecurityRealtimeWSAmbiguousKeysNeverForwarded(t *testing.T) {
	for name, frame := range map[string]string{
		"type-duplicate":         `{"type":"session.update","type":"conversation.item.create","session":{"model":"gpt-realtime-2"}}`,
		"nested-model-duplicate": `{"type":"session.update","session":{"model":"gpt-realtime-2","model":"gpt-4o-realtime-preview"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			received := make(chan string, 4)
			upstream := newRecordingRealtimeUpstream(t, received)
			defer upstream.Close()
			proxy := newRealtimeProxyTestServer(t, upstream.URL)
			defer proxy.Close()

			conn, resp, err := websocket.DefaultDialer.Dial(strings.Replace(proxy.URL, "http://", "ws://", 1)+"/v1/realtime?model=gpt-4o-realtime-preview", nil)
			require.NoError(t, err, "the local realtime proxy must accept a WebSocket upgrade")
			require.NoError(t, resp.Body.Close())
			defer func() { _ = conn.Close() }()
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(frame)))

			require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
			var lastEvent map[string]any
			var closeErr *websocket.CloseError
			for {
				_, msg, readErr := conn.ReadMessage()
				if readErr != nil {
					errors.As(readErr, &closeErr)
					break
				}
				var event map[string]any
				if json.Unmarshal(msg, &event) == nil {
					lastEvent = event
				}
			}
			require.NotNil(t, closeErr, "the socket must close on an ambiguous session.update")
			require.NotNil(t, lastEvent)
			require.Equal(t, "error", lastEvent["type"])
			select {
			case got := <-received:
				t.Fatalf("provider received an ambiguous frame: %q", got)
			case <-time.After(200 * time.Millisecond):
			}
		})
	}
}
