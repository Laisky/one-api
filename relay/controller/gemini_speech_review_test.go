package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// TestGeminiSpeechReviewSettlementHTTP checks truncated output and internal codec credits end to end.
// Parameters: t is the test handle. Returns: none. The shared database fixture is intentionally serial.
func TestGeminiSpeechReviewSettlementHTTP(t *testing.T) {
	const actual = "gemini-3.8-flash-tts"
	pcm := bytes.Repeat([]byte{1, 2}, 1920)
	for _, tc := range []struct {
		name        string
		format      string
		stream      bool
		truncated   bool
		encoderFail bool
		writeFail   bool
		unlimited   bool
		balance     int64
		charge      int64
	}{
		{name: "max_tokens_wav", format: "wav", truncated: true, balance: 10000, charge: 12},
		{name: "max_tokens_buffered_sse", format: "wav", stream: true, truncated: true, balance: 10000, charge: 12},
		{name: "max_tokens_incremental_sse", format: "pcm", stream: true, truncated: true, balance: 10000, charge: 12},
		{name: "encoder_credit_prepaid", format: "mp3", encoderFail: true, balance: 10000},
		{name: "encoder_credit_buffered_sse", format: "mp3", stream: true, encoderFail: true, balance: 10000},
		{name: "encoder_credit_trusted", format: "mp3", encoderFail: true, balance: 10000000},
		{name: "encoder_credit_unlimited", format: "mp3", encoderFail: true, unlimited: true, balance: 10000},
		{name: "truncated_client_disconnect", format: "wav", truncated: true, writeFail: true, balance: 10000, charge: 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, tc.balance, tc.unlimited)
			if tc.encoderFail {
				// LookPath succeeds, but exec rejects this deliberately invalid executable.
				// This exercises the real post-provider encoder failure, without a shell,
				// downloaded codec, process-global test hook, or missing-PATH false positive.
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte("invalid executable fixture\n"), 0o700))
				t.Setenv("PATH", dir)
			}
			finish := "STOP"
			if tc.truncated {
				finish = "MAX_TOKENS"
			}
			payload := fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/L16;rate=24000","data":%q}}]},"finishReason":%q}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":2,"cachedContentTokenCount":4}}`, base64.StdEncoding.EncodeToString(pcm), finish)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				body := payload
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					body = "data: " + payload + "\n\n"
				}
				_, _ = io.WriteString(w, body)
			}))
			t.Cleanup(server.Close)
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			request := map[string]any{"model": "alias", "input": "Exact transcript.", "voice": "voicekey_private-fixture", "response_format": tc.format, "gemini": map[string]int{"max_output_tokens": 2}}
			if tc.stream {
				request["stream_format"] = "sse"
			}
			body, err := json.Marshal(request)
			require.NoError(t, err)
			c, w, id := protocolContext(t, channeltype.Gemini, actual, "/v1/audio/speech", string(body), server.URL, tc.balance, 1, tc.unlimited, nil)
			meta.GetByContext(c).StartTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
			if tc.writeFail {
				c.Writer = xaiDisconnectedWriter{ResponseWriter: c.Writer}
			}
			apiErr := RelayAudioHelper(c, relaymode.AudioSpeech)
			drainCriticalTasks(t)
			require.Equal(t, int32(1), calls.Load())
			require.True(t, c.GetBool(adaptor.AudioReceiptAcceptedKey), "customer credit must not enable replay of paid work")
			require.Equal(t, tc.balance-tc.charge, reloadUserQuota(t))
			require.Equal(t, tc.charge, requestCostQuota(t, id))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, tc.balance-tc.charge, token.RemainQuota)
			require.Equal(t, tc.charge, token.UsedQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ?", id).Find(&logs).Error)
			require.Len(t, logs, 1, "settlement must reconcile, not duplicate, the provisional row")
			entry := logs[0]
			require.Equal(t, model.LogTypeConsume, entry.Type)
			require.EqualValues(t, tc.charge, entry.Quota)
			require.Equal(t, 12, entry.PromptTokens)
			require.Equal(t, 2, entry.CompletionTokens)
			require.Equal(t, 4, entry.CachedPromptTokens)
			require.Contains(t, entry.Content, fmt.Sprintf("truncated=%t", tc.truncated))
			require.Contains(t, entry.Content, fmt.Sprintf("encoding_failed_refund=%t", tc.encoderFail))
			if tc.encoderFail || tc.writeFail {
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
				require.NotContains(t, apiErr.Message, "voicekey_private-fixture")
				if tc.encoderFail {
					require.False(t, c.Writer.Written())
					require.Empty(t, w.Body.Bytes())
					var user model.User
					require.NoError(t, model.DB.First(&user, c.GetInt(ctxkey.Id)).Error)
					require.Zero(t, user.UsedQuota)
				}
				return
			}
			require.Nil(t, apiErr)
			if tc.stream {
				require.Equal(t, 1, strings.Count(w.Body.String(), "event: speech.audio.done"))
				require.Contains(t, w.Body.String(), `"truncated":true`)
				require.NotContains(t, w.Body.String(), "event: error")
			} else {
				require.Equal(t, "MAX_TOKENS", w.Result().Header.Get("X-Gemini-Finish-Reason"))
				require.Equal(t, "RIFF", w.Body.String()[:4])
				require.Equal(t, uint32(len(pcm)), binary.LittleEndian.Uint32(w.Body.Bytes()[40:44]))
				require.Equal(t, pcm, w.Body.Bytes()[44:])
			}
		})
	}
}
