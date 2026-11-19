package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestProviderPrefixedModelKeepsChatControls drives Chat Completions through an
// OpenAI-compatible channel whose mapped upstream ID carries a provider prefix
// ("openai/gpt-4o-mini", the aggregator naming). The OpenAI o-series heuristic
// must not classify such IDs as reasoning models: doing so silently dropped the
// client's system prompt and rewrote its sampling controls. Bare o-series IDs
// (the control) keep their historical reasoning normalization.
func TestProviderPrefixedModelKeepsChatControls(t *testing.T) {
	for _, tc := range []struct {
		actual          string
		wantSystem      bool
		wantTemperature bool
	}{
		{actual: "openai/gpt-4o-mini", wantSystem: true, wantTemperature: true},
		{actual: "openai/gpt-oss-120b", wantSystem: true, wantTemperature: true},
		{actual: "o3-mini", wantSystem: false, wantTemperature: false},
	} {
		t.Run(tc.actual, func(t *testing.T) {
			xaiVideoSetup(t, 1_000_000, false)
			bodies := make(chan []byte, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				bodies <- raw
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"chatcmpl-prefixed","object":"chat.completion","model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)
			}))
			t.Cleanup(upstream.Close)
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = previous })

			body := `{"model":"alias","messages":[{"role":"system","content":"Answer in French."},{"role":"user","content":"hello"}],"temperature":0.3,"max_tokens":32}`
			c, _, _ := protocolContext(t, channeltype.Custom, tc.actual, "/v1/chat/completions", body, upstream.URL, 1_000_000, 1, false, nil)
			require.Nil(t, RelayTextHelper(c))
			drainCriticalTasks(t)

			var sent struct {
				Messages []struct {
					Role string `json:"role"`
				} `json:"messages"`
				Temperature *float64 `json:"temperature"`
			}
			require.NoError(t, json.Unmarshal(<-bodies, &sent))
			hasSystem := false
			for _, message := range sent.Messages {
				hasSystem = hasSystem || message.Role == "system"
			}
			require.Equal(t, tc.wantSystem, hasSystem, "system prompt forwarded")
			if tc.wantTemperature {
				require.NotNil(t, sent.Temperature)
				require.InDelta(t, 0.3, *sent.Temperature, 1e-9, "client temperature must be preserved")
			} else if sent.Temperature != nil {
				require.NotEqual(t, 0.3, *sent.Temperature, "control: o-series sampling is normalized")
			}
		})
	}
}
