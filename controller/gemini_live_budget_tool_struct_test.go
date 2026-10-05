package controller

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// liveToolSetup declares the client-executed function the provider calls.
const liveToolSetup = `{"setup":{"model":"` + liveBudgetModel + `","tools":[{"functionDeclarations":[{"name":"lookup","parameters":{"type":"OBJECT","properties":{"query":{"type":"STRING"}}}}]}]}}`

// liveMediaNamedSchemaSetup declares parameters whose names collide with Part
// media fields; a Schema's property names are application data, not media.
const liveMediaNamedSchemaSetup = `{"setup":{"model":"` + liveBudgetModel + `","tools":[{"functionDeclarations":[{"name":"lookup","parameters":{"type":"OBJECT","properties":{"inlineData":{"type":"STRING"},"fileData":{"type":"STRING"}}}}]}]}}`

// liveToolResponse builds a native function result. Parameters: body is the
// FunctionResponse fields after id and name. Returns: the client frame.
func liveToolResponse(body string) string {
	return `{"toolResponse":{"functionResponses":[{"id":"call-1","name":"lookup",` + body + `}]}}`
}

// liveTinyPNG returns a valid base64 PNG. Parameters: t owns assertions.
// Returns: image data for a genuine FunctionResponse media part.
func liveTinyPNG(t *testing.T) string {
	t.Helper()
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 8, 8))))
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}

// TestSecurityGeminiLiveFunctionResultStructIsText reproduces the review on
// PR #530: FunctionResponse.response is an arbitrary google.protobuf.Struct,
// so keys named inlineData or fileData inside it are ordinary text context,
// not media. Only FunctionResponse.parts[].inlineData carries media. Each case
// answers a provider-issued call at physical balances around the real text
// allowance and asserts what the provider received. Parameters: t owns the test.
func TestSecurityGeminiLiveFunctionResultStructIsText(t *testing.T) {
	// The open function-call turn and the turn its result funds need about
	// 42,000 quota of allowances and context; 50,000 leaves ~8,000 for input,
	// i.e. ~21 KB of text at 0.375 quota per (one-byte) token.
	const balance = 50_000
	pcmSecond := base64.StdEncoding.EncodeToString(make([]byte, 32000))
	for _, tc := range []struct {
		name      string
		setup     string
		response  string
		forwarded bool
		reason    string
	}{
		{"schema_property_names_are_not_media", liveMediaNamedSchemaSetup,
			liveToolResponse(`"response":{"ok":true}`), true, ""},
		{"smuggled_text_beyond_allowance_is_refused", liveToolSetup,
			liveToolResponse(`"response":{"inlineData":{"mimeType":"audio/pcm;rate=16000","data":"` + strings.Repeat("x", 40_000) + `"}}`),
			false, liveQuotaCloseReason},
		{"struct_keys_within_allowance_are_forwarded_unchanged", liveToolSetup,
			liveToolResponse(`"response":{"inlineData":{"mimeType":"audio/pcm;rate=16000","data":"` + strings.Repeat("x", 8_000) + `"},"fileData":{"fileUri":"not-a-provider-fetch"}}`),
			true, ""},
		{"genuine_audio_part_is_priced_as_audio", liveToolSetup,
			// Priced as text these 42,668 bytes would need ~16,000 quota.
			liveToolResponse(`"response":{"ok":true},"parts":[{"inlineData":{"mimeType":"audio/pcm;rate=16000","data":"` + pcmSecond + `"}}]`),
			true, ""},
		{"genuine_image_part_is_priced_as_image", liveToolSetup,
			liveToolResponse(`"response":{"ok":true},"parts":[{"inline_data":{"mime_type":"image/png","data":"` + liveTinyPNG(t) + `"}}]`),
			true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan string, 1)
			env := newLiveBudgetEnv(t, liveBudgetOptions{UserQuota: balance, TokenQuota: balance, Group: 1},
				func(conn *websocket.Conn) error {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"toolCall":{"functionCalls":[{"id":"call-1","name":"lookup","args":{}}]}}`)); err != nil {
						return errors.Wrap(err, "issue function call")
					}
					_, raw, err := conn.ReadMessage()
					if err != nil {
						received <- ""
						return nil
					}
					received <- string(raw)
					_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second))
					return nil
				})
			conn := env.connectSetup(tc.setup)
			_, call, err := conn.ReadMessage()
			require.NoError(t, err)
			require.Contains(t, string(call), "call-1")
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(tc.response)))
			reason := awaitClose(conn, 3*time.Second)
			env.finish()

			got := <-received
			userQuota, tokenQuota := env.balances()
			t.Logf("forwarded=%v close=%q user_quota=%d", got != "", reason, userQuota)
			// Compare without dumping multi-kilobyte payloads into failure output.
			if tc.forwarded {
				require.True(t, got == tc.response, "a funded function result reaches the provider unchanged (got %d bytes)", len(got))
				require.NotEqual(t, liveQuotaCloseReason, reason)
			} else {
				require.Zero(t, len(got), "the provider must not receive underfunded function-result text")
				require.Equal(t, tc.reason, reason)
			}
			require.GreaterOrEqual(t, userQuota, int64(0))
			require.GreaterOrEqual(t, tokenQuota, int64(0))
			for _, rows := range env.consumeLogs() {
				require.Len(t, rows, 1, "the session settles exactly once")
			}
		})
	}
}
