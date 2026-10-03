package controller

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiSpeechHTTP exercises real request conversion, authentication, and all billing ledgers.
func TestGeminiSpeechHTTP(t *testing.T) {
	const actual = "gemini-3.8-flash-tts"
	pcm := make([]byte, 3840)
	good := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/L16;rate=24000","data":"` + base64.StdEncoding.EncodeToString(pcm) + `"}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":2,"cachedContentTokenCount":4}}`
	for _, tc := range []struct {
		name                         string
		status                       int
		body                         string
		charge                       int64
		channel                      int
		balance                      int64
		writeFail, stream, unlimited bool
		local                        *model.ModelConfigLocal
	}{
		{name: "native", status: 200, body: good, charge: 12, channel: channeltype.Gemini, balance: 10000},
		{name: "compatible", status: 200, body: good, charge: 12, channel: channeltype.GeminiOpenAICompatible, balance: 10000},
		{name: "trusted_reservation_skip", status: 200, body: good, charge: 12, channel: channeltype.Gemini, balance: 10000000},
		{name: "unlimited_token", status: 200, body: good, charge: 12, channel: channeltype.Gemini, balance: 10000, unlimited: true},
		{name: "client_disconnect", status: 200, body: good, charge: 12, channel: channeltype.Gemini, balance: 10000, writeFail: true},
		{name: "stream", status: 200, body: good, charge: 12, channel: channeltype.Gemini, balance: 10000, stream: true},
		{name: "partial_stream", status: 200, body: strings.Replace(good, `"STOP"`, `""`, 1), charge: 12, channel: channeltype.Gemini, balance: 10000, stream: true},
		{name: "refused", status: 400, body: `{"error":{"message":"private-input"}}`, channel: channeltype.Gemini, balance: 10000},
		{name: "malformed", status: 200, body: `{"candidates":[]}`, channel: channeltype.Gemini, balance: 10000},
		{name: "low_balance", status: 200, body: good, channel: channeltype.Gemini, balance: 1},
		{name: "override", status: 200, body: good, charge: 40, channel: channeltype.Gemini, balance: 50000, local: &model.ModelConfigLocal{Ratio: 2, CompletionRatio: 8, CachedInputRatio: -1, Audio: &model.AudioPricingLocal{PromptRatio: 1, CompletionRatio: 6}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, tc.balance, tc.unlimited)
			var calls atomic.Int32
			observed := make(chan map[string]any, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var native map[string]any
				err := json.NewDecoder(r.Body).Decode(&native)
				observed <- map[string]any{"decode_error": err, "body": native, "path": r.URL.Path, "query": r.URL.RawQuery, "key": r.Header.Get("x-goog-api-key"), "authorization": r.Header.Get("Authorization")}
				contentType := "application/json"
				body := tc.body
				if tc.stream {
					contentType = "text/event-stream"
					body = "data: " + body + "\n\n"
				}
				w.Header().Set("Content-Type", contentType)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()
			body := `{"model":"alias","input":"Exact transcript.","voice":"voicekey_private-fixture","instructions":"calm","response_format":"pcm","extra_body":{"gemini":{"max_output_tokens":10}}}`
			if tc.stream {
				body = strings.Replace(body, `"response_format":"pcm"`, `"response_format":"pcm","stream_format":"sse"`, 1)
			}
			c, w, id := protocolContext(t, tc.channel, actual, "/v1/audio/speech", body, server.URL+"/prefix/v1beta/openai", tc.balance, 1, tc.unlimited, tc.local)
			meta.GetByContext(c).StartTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
			if tc.writeFail {
				c.Writer = xaiDisconnectedWriter{ResponseWriter: c.Writer}
			}
			apiErr := RelayAudioHelper(c, relaymode.AudioSpeech)
			drainCriticalTasks(t)
			require.Equal(t, tc.balance-tc.charge, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if !tc.unlimited {
				require.Equal(t, tc.balance-tc.charge, token.RemainQuota)
				require.Equal(t, tc.charge, token.UsedQuota)
			}
			if tc.balance == 1 {
				require.NotNil(t, apiErr)
				require.Zero(t, calls.Load())
				return
			}
			require.Equal(t, tc.charge, requestCostQuota(t, id))
			require.Equal(t, int32(1), calls.Load())
			wire := <-observed
			require.Nil(t, wire["decode_error"])
			action := ":generateContent"
			query := ""
			if tc.stream {
				action = ":streamGenerateContent"
				query = "alt=sse"
			}
			require.Equal(t, "/prefix/v1beta/models/"+actual+action, wire["path"])
			require.Equal(t, query, wire["query"])
			require.Empty(t, wire["authorization"])
			require.Equal(t, "upstream-fixture-key", wire["key"])
			native := wire["body"].(map[string]any)
			parts := native["contents"].([]any)[0].(map[string]any)["parts"].([]any)
			require.Equal(t, "Exact transcript.", parts[0].(map[string]any)["text"])
			require.NotContains(t, native, "input")
			require.NotContains(t, native, "extra_body")
			if tc.charge > 0 {
				require.True(t, c.GetBool(adaptor.AudioReceiptAcceptedKey))
			}
			if tc.status != 200 || tc.writeFail || tc.name == "malformed" || tc.name == "partial_stream" {
				require.NotNil(t, apiErr)
				require.NotContains(t, apiErr.Message, "private-input")
			} else {
				require.Nil(t, apiErr)
				if tc.stream {
					require.Contains(t, w.Body.String(), "speech.audio.done")
				} else {
					require.Equal(t, pcm, w.Body.Bytes())
				}
			}
			require.NotContains(t, w.Body.String(), "voicekey_private-fixture")
		})
	}
}

// TestGeminiSpeechInvalidRequestDoesNotReserve exercises validation before quota or upstream side effects.
func TestGeminiSpeechInvalidRequestDoesNotReserve(t *testing.T) {
	const balance = int64(10000)
	for _, body := range []string{
		`{"model":"alias","input":"hello","voice":"Kore","response_format":"bad"}`,
		`{"model":"alias","input":"hello","voice":"Kore","response_format":"pcm","speed":0}`,
		`{"model":"alias","input":"hello","voice":"Kore","response_format":"pcm","extra_body":{"input":"hidden text"}}`,
	} {
		xaiVideoSetup(t, balance, false)
		c, _, _ := protocolContext(t, channeltype.Gemini, "gemini-3.8-flash-tts", "/v1/audio/speech", body, "https://must-not-be-contacted.invalid", balance, 1, false, nil)
		apiErr := RelayAudioHelper(c, relaymode.AudioSpeech)
		require.NotNil(t, apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		drainCriticalTasks(t)
		require.Equal(t, balance, reloadUserQuota(t))
		require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
	}
}

// TestGeminiSpeechURL checks native/compatible bases, explicit endpoint overrides, and credential rejection.
func TestGeminiSpeechURL(t *testing.T) {
	t.Parallel()
	for _, base := range []string{"https://example.test", "https://example.test/v1beta", "https://example.test/v1beta/openai"} {
		m := &meta.Meta{Mode: relaymode.AudioSpeech, ChannelType: channeltype.Gemini, BaseURL: base, ActualModelName: "gemini-3.8-flash-tts"}
		actual, err := geminiSpeechURL(m, true)
		require.NoError(t, err)
		require.Equal(t, "https://example.test/v1beta/models/gemini-3.8-flash-tts:streamGenerateContent?alt=sse", actual)
		m.Config.EndpointURLs = map[string]string{"audio_speech": "https://private-proxy.test/tts"}
		actual, err = geminiSpeechURL(m, false)
		require.NoError(t, err)
		require.Equal(t, "https://private-proxy.test/tts", actual)
	}
	for _, base := range []string{"https://user:secret@example.test", "https://example.test?key=secret", "file:///tmp/voice", "https://example.test#secret"} {
		_, err := geminiSpeechURL(&meta.Meta{BaseURL: base, ActualModelName: "gemini-3.8-flash-tts"}, false)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}
