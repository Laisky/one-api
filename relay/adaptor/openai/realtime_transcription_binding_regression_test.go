package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestRealtimeTranscriptionBindingWire drives the real handler against a local
// recording upstream. t supplies assertions and cleanup; the test returns nothing.
func TestRealtimeTranscriptionBindingWire(t *testing.T) {
	for _, tc := range []struct {
		name, frame                       string
		deny, binary, conversation, alias bool
	}{
		{name: "GA nested model", frame: `{"type":"session.update","session":{"type":"transcription","audio":{"input":{"transcription":{"model":"expensive"}}}}}`, deny: true},
		{name: "GA update without type", frame: `{"type":"session.update","session":{"audio":{"input":{"transcription":{"model":"expensive"}}}}}`, deny: true},
		{name: "legacy nested model", frame: `{"type":"transcription_session.update","session":{"input_audio_transcription":{"model":"expensive"}}}`, deny: true},
		{name: "legacy root model", frame: `{"type":"transcription_session.update","input_audio_transcription":{"model":"expensive"}}`, deny: true},
		{name: "session update legacy field", frame: `{"type":"session.update","session":{"input_audio_transcription":{"model":"expensive"}}}`, deny: true},
		{name: "binary JSON cannot bypass", frame: `{"type":"session.update","session":{"audio":{"input":{"transcription":{"model":"expensive"}}}}}`, deny: true, binary: true},
		{name: "bound primary cannot hide nested switch", frame: `{"type":"session.update","session":{"model":"gpt-4o-mini-transcribe","audio":{"input":{"transcription":{"model":"expensive"}}}}}`, deny: true},
		{name: "null transcription model", frame: `{"type":"session.update","session":{"audio":{"input":{"transcription":{"model":null}}}}}`, deny: true},
		{name: "non-string transcription model", frame: `{"type":"session.update","session":{"audio":{"input":{"transcription":{"model":42}}}}}`, deny: true},
		{name: "alias preserves numeric precision", frame: `{"type":"session.update","counter":9007199254740993,"session":{"audio":{"input":{"transcription":{"model":"speech-alias","prompt":"technical terms"}}}}}`, alias: true},
		{name: "bound transcription allowed", frame: `{"type":"session.update","session":{"audio":{"input":{"transcription":{"model":"gpt-4o-mini-transcribe"}}}}}`},
		{name: "configuration without model", frame: `{"type":"session.update","session":{"audio":{"input":{"transcription":{"language":"en"}}}}}`},
		{name: "conversation auxiliary model is distinct", frame: `{"type":"session.update","session":{"type":"realtime","model":"gpt-realtime","audio":{"input":{"transcription":{"model":"whisper-1"}}}}}`, conversation: true},
		{name: "transcription cannot become conversation", frame: `{"type":"session.update","session":{"type":"realtime"}}`, deny: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan string, 4)
			closed := make(chan struct{})
			upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					close(closed)
					return
				}
				defer conn.Close()
				defer close(closed)
				for {
					_, msg, err := conn.ReadMessage()
					if err != nil {
						return
					}
					received <- string(msg)
					if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"forwarded"}`)); err != nil {
						return
					}
				}
			}))
			defer upstream.Close()
			bound := "gpt-4o-mini-transcribe"
			query := "?model=speech-alias&intent=transcription"
			if tc.conversation {
				bound = "gpt-realtime"
				query = "?model=speech-alias"
			}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/v1/realtime", func(c *gin.Context) {
				_, _ = RealtimeHandler(c, &rmeta.Meta{Mode: relaymode.Realtime, BaseURL: upstream.URL, APIKey: "test-key", ActualModelName: bound, OriginModelName: "speech-alias"})
			})
			proxy := httptest.NewServer(router)
			defer proxy.Close()
			conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(proxy.URL, "http:", "ws:", 1)+"/v1/realtime"+query, nil)
			require.NoError(t, err)
			defer conn.Close()
			kind := websocket.TextMessage
			if tc.binary {
				kind = websocket.BinaryMessage
			}
			require.NoError(t, conn.WriteMessage(kind, []byte(tc.frame)))
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
			_, reply, err := conn.ReadMessage()
			require.NoError(t, err)
			var event struct {
				Type  string `json:"type"`
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(reply, &event))
			if tc.deny {
				require.Equal(t, "error", event.Type, "forbidden frames must never reach upstream")
				require.Equal(t, "model_switch_denied", event.Error.Code)
				_, _, err = conn.ReadMessage()
				require.True(t, websocket.IsCloseError(err, websocket.ClosePolicyViolation))
				select {
				case <-closed:
				case <-time.After(3 * time.Second):
					t.Fatal("upstream did not close")
				}
				require.Empty(t, received)
				return
			}
			require.Equal(t, "forwarded", event.Type)
			select {
			case got := <-received:
				if tc.alias {
					require.NotContains(t, got, "speech-alias")
					require.Contains(t, got, `"model":"`+bound+`"`)
					require.Contains(t, got, `"counter":9007199254740993`)
					require.Contains(t, got, "technical terms")
				} else {
					require.JSONEq(t, tc.frame, got)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("authorized frame was not forwarded")
			}
		})
	}
}
