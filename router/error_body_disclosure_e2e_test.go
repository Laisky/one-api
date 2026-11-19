package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/channeltype"
)

// errorBodyDisclosureSecret stands for an internal host or account identifier
// that a failing provider's error body can carry (OWASP API8:2023 / API10:2023).
const errorBodyDisclosureSecret = "internal-account-123"

// TestPublicRelayErrorBodyNeverEchoed sends a chat request through the shipped
// router to a provider that fails with an unparseable or unrecognized body and
// requires the client to receive the provider status with the generic message,
// never the raw body. HTTP 502 keeps the message visible (the relay rewrites
// client-facing 429 messages).
func TestPublicRelayErrorBodyNeverEchoed(t *testing.T) {
	for _, tc := range []struct{ name, contentType, body, want string }{
		{"html", "text/html; charset=utf-8", `<html>internal-host-10.0.0.7 ` + errorBodyDisclosureSecret + `</html>`,
			"upstream returned HTTP 502 with an unrecognized error body (content-type text/html, %d bytes)"},
		{"json_unknown_keys", "application/json", `{"private_payload":"` + errorBodyDisclosureSecret + `"}`,
			"upstream returned HTTP 502 with an unrecognized error body (content-type application/json, %d bytes)"},
		{"oversized", "text/plain", strings.Repeat(errorBodyDisclosureSecret, (2<<20)/len(errorBodyDisclosureSecret)),
			"upstream returned HTTP 502 with an error body larger than the 1048576-byte limit (content-type text/plain)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(provider.Close)
			engine, key := ambiguousKeysRouter(t, provider.URL, channeltype.OpenAICompatible, passthroughKeysAllowedModel, true)
			client.HTTPClient = provider.Client()
			client.HTTPClient.Timeout = 5 * time.Second
			oldRetries, oldBatch := config.RetryTimes, config.BatchUpdateEnabled
			config.RetryTimes, config.BatchUpdateEnabled = 0, false
			t.Cleanup(func() { config.RetryTimes, config.BatchUpdateEnabled = oldRetries, oldBatch })

			w := postChatCompletion(t, engine, key, `{"model":"`+passthroughKeysAllowedModel+`","messages":[{"role":"user","content":"hi"}]}`)
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
			require.NotContains(t, w.Body.String(), errorBodyDisclosureSecret, "the client must not receive the raw provider body")
			var envelope struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
			want := tc.want
			if strings.Contains(want, "%d") {
				want = fmt.Sprintf(want, len(tc.body))
			}
			require.Contains(t, envelope.Error.Message, want)
		})
	}
}
