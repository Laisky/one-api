package openai

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	rmeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestRealtimeUpstreamQueryForwardsOnlyDocumentedParameters verifies the
// realtime handshake forwards only the query parameters OpenAI documents for
// GET /v1/realtime that the gateway can meter: `model` (always the gateway's
// mapped model) and `intent` (the single value the gateway metered as).
// `call_id` attaches to an existing WebRTC/SIP call that this gateway never
// created, so it is dropped together with every undocumented parameter.
// Parameters: t is the test handle. Returns: none.
func TestRealtimeUpstreamQueryForwardsOnlyDocumentedParameters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		clientQuery string
		model       string
		wantQuery   url.Values
	}{
		{
			name:        "call_id_sideband_attach_is_dropped",
			clientQuery: "model=my-voice&call_id=rtc_victim_call",
			model:       "gpt-realtime-mini",
			wantQuery:   url.Values{"model": {"gpt-realtime-mini"}},
		},
		{
			name:        "provider_selectors_and_case_variants_are_dropped",
			clientQuery: "model=my-voice&Model=gpt-realtime&MODEL=gpt-realtime&deployment=gpt-realtime&api-version=2025-04-01-preview&api-key=sk-caller",
			model:       "gpt-realtime-mini",
			wantQuery:   url.Values{"model": {"gpt-realtime-mini"}},
		},
		{
			name:        "duplicate_intent_forwards_the_metered_conversation_value",
			clientQuery: "model=my-voice&intent=realtime&intent=transcription",
			model:       "gpt-realtime-mini",
			wantQuery:   url.Values{"model": {"gpt-realtime-mini"}, "intent": {"realtime"}},
		},
		{
			name:        "duplicate_intent_forwards_the_metered_transcription_value",
			clientQuery: "model=gpt-4o-transcribe&intent=transcription&intent=realtime",
			model:       "gpt-4o-transcribe",
			wantQuery:   url.Values{"intent": {"transcription"}},
		},
		{
			name:        "padded_transcription_intent_is_canonicalized",
			clientQuery: "model=gpt-4o-transcribe&intent=%20TRANSCRIPTION%20",
			model:       "gpt-4o-transcribe",
			wantQuery:   url.Values{"intent": {"transcription"}},
		},
		{
			name:        "duplicate_model_values_collapse_to_the_mapped_model",
			clientQuery: "model=my-voice&model=gpt-realtime",
			model:       "gpt-realtime-mini",
			wantQuery:   url.Values{"model": {"gpt-realtime-mini"}},
		},
		{
			name:        "empty_intent_is_not_forwarded",
			clientQuery: "model=my-voice&intent=",
			model:       "gpt-realtime-mini",
			wantQuery:   url.Values{"model": {"gpt-realtime-mini"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, _, err := realtimeWebSocketUpstreamURL(&rmeta.Meta{ActualModelName: tc.model}, tc.clientQuery)
			require.NoError(t, err)
			parsed, err := url.Parse(raw)
			require.NoError(t, err)
			require.Equal(t, "wss", parsed.Scheme)
			require.Equal(t, "api.openai.com", parsed.Host)
			require.Equal(t, "/v1/realtime", parsed.Path)
			require.Equal(t, tc.wantQuery, parsed.Query())
		})
	}
}

// TestRealtimeUpstreamQueryCannotRedirectHandshake is the control for the
// go/request-forgery review: whatever the caller puts in the query string, the
// upstream scheme, host, credentials and path come only from the channel
// configuration. Parameters: t is the test handle. Returns: none.
func TestRealtimeUpstreamQueryCannotRedirectHandshake(t *testing.T) {
	t.Parallel()
	for _, query := range []string{
		"model=x&@evil.example/",
		"model=x#@evil.example/path",
		"model=x&%2F..%2F..%2Fadmin=1",
		"model=x&next=//evil.example/v1/realtime",
		"model=x&\\\\evil.example=1",
		"model=x&user:pass@evil.example=1",
		"%zz=1&model=x",
		"model=x;intent=transcription",
	} {
		for _, override := range []string{"", "https://override.example/custom/realtime"} {
			raw, _, err := realtimeWebSocketUpstreamURL(realtimeMeta("https://channel.example", override, "gpt-realtime"), query)
			require.NoError(t, err, "query=%q", query)
			parsed, err := url.Parse(raw)
			require.NoError(t, err, "query=%q", query)
			require.Equal(t, "wss", parsed.Scheme, "query=%q", query)
			require.Nil(t, parsed.User, "query=%q", query)
			require.Empty(t, parsed.Fragment, "query=%q", query)
			if override == "" {
				require.Equal(t, "channel.example", parsed.Host, "query=%q", query)
				require.Equal(t, "/v1/realtime", parsed.Path, "query=%q", query)
			} else {
				require.Equal(t, "override.example", parsed.Host, "query=%q", query)
				require.Equal(t, "/custom/realtime", parsed.Path, "query=%q", query)
			}
			require.Equal(t, "gpt-realtime", parsed.Query().Get("model"), "query=%q", query)
		}
	}
}

// TestRealtimeHandshakeDoesNotForwardCallerQuery reproduces the forwarding
// end to end: a caller connects through RealtimeHandler with a sideband
// `call_id`, case-variant `Model`, and provider selector parameters, and a
// fake provider records the handshake it receives with the channel credential.
// Only the gateway-controlled model may reach the provider. Parameters: t is
// the test handle. Returns: none.
func TestRealtimeHandshakeDoesNotForwardCallerQuery(t *testing.T) {
	t.Parallel()
	type handshake struct {
		rawQuery      string
		authorization string
	}
	seen := make(chan handshake, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- handshake{rawQuery: r.URL.RawQuery, authorization: r.Header.Get("Authorization")}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		c.Writer = &ginResponseWriter{w: w, ResponseWriter: c.Writer}
		meta := &rmeta.Meta{
			Mode:            relaymode.Realtime,
			BaseURL:         upstream.URL,
			APIKey:          "sk-channel-fixture",
			ActualModelName: "gpt-realtime-mini",
			OriginModelName: "my-voice",
		}
		_, _ = RealtimeHandler(c, meta)
	}))
	defer gateway.Close()

	clientURL := strings.Replace(gateway.URL, "http://", "ws://", 1) +
		"/v1/realtime?model=my-voice&call_id=rtc_victim_call&Model=gpt-realtime&deployment=gpt-realtime&api-version=2025-04-01-preview"
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	clientConn, response, err := dialer.Dial(clientURL, nil)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	require.NoError(t, err)
	defer clientConn.Close()

	select {
	case got := <-seen:
		require.Equal(t, "Bearer sk-channel-fixture", got.authorization)
		forwarded, parseErr := url.ParseQuery(got.rawQuery)
		require.NoError(t, parseErr)
		require.Equal(t, url.Values{"model": {"gpt-realtime-mini"}}, forwarded,
			"the provider handshake must carry only the gateway-controlled model")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "provider handshake was not observed")
	}
	require.NoError(t, clientConn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")))
}

// TestCallerHandshakeQueryUnforwardedCount verifies the DEBUG diagnostic count
// covers dropped pairs, rewritten values and discarded malformed pairs, while
// a forwarded pair is not counted. Parameters: t is the test handle.
// Returns: none.
func TestCallerHandshakeQueryUnforwardedCount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		query string
		want  int
	}{
		{query: "model=gpt-realtime", want: 0},
		{query: "model=my-voice&call_id=rtc_1&trace=a&trace=b", want: 4},
		{query: "model=gpt-realtime&%zz=1", want: 1},
	} {
		_, dropped, err := realtimeWebSocketUpstreamURL(&rmeta.Meta{ActualModelName: "gpt-realtime"}, tc.query)
		require.NoError(t, err)
		require.Equal(t, tc.want, dropped, "query=%q", tc.query)
	}
}
