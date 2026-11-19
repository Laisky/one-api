package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestChatRawPassthroughHonorsResponsesWireFormat pins the raw Chat
// passthrough (ENFORCE_INCLUDE_USAGE=false) to Chat-wire upstreams. Azure
// GPT-5 deployments and OpenAI-compatible channels configured for the
// Responses format are called on their Responses endpoint, so the Chat body
// must be converted there; forwarding it raw sent a Chat body ("messages") to
// /responses, which the provider rejects.
func TestChatRawPassthroughHonorsResponsesWireFormat(t *testing.T) {
	previous := config.EnforceIncludeUsage
	config.EnforceIncludeUsage = false
	t.Cleanup(func() { config.EnforceIncludeUsage = previous })
	for _, tc := range []struct {
		name, model string
		channel     int
		cfg         model.ChannelConfig
	}{
		{name: "azure_gpt5", model: "gpt-5-mini", channel: channeltype.Azure, cfg: model.ChannelConfig{APIVersion: "2025-04-01-preview"}},
		{name: "compatible_responses_format", model: "gpt-4o-mini", channel: channeltype.OpenAICompatible, cfg: model.ChannelConfig{APIFormat: "response"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10_000_000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			var gotPath atomic.Value
			var gotBody atomic.Value
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				raw, _ := io.ReadAll(r.Body)
				gotPath.Store(r.URL.Path)
				gotBody.Store(string(raw))
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/responses") {
					_, _ = io.WriteString(w, `{"id":"resp_wire","object":"response","status":"completed","model":"`+tc.model+`","output":[{"type":"message","id":"msg_wire","role":"assistant","content":[{"type":"output_text","text":"ack"}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}`)
					return
				}
				_, _ = io.WriteString(w, `{"id":"chatcmpl-wire","object":"chat.completion","created":1,"model":"`+tc.model+`","choices":[{"index":0,"message":{"role":"assistant","content":"ack"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
			}))
			t.Cleanup(upstream.Close)
			previousClient := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = previousClient })

			body := `{"model":"` + tc.model + `","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`
			c, _, _ := protocolContext(t, tc.channel, tc.model, "/v1/chat/completions", body, upstream.URL, balance, 1, false, nil)
			// Keep the client model name so the raw passthrough branch is eligible.
			c.Set(ctxkey.ModelMapping, map[string]string{})
			c.Set(ctxkey.RequestModel, tc.model)
			c.Request.Body = io.NopCloser(strings.NewReader(body))
			c.Set(ctxkey.Config, tc.cfg)
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, calls.Load())
			path, _ := gotPath.Load().(string)
			require.True(t, strings.HasSuffix(path, "/responses"), "control: the route targets Responses, got %q", path)
			var sent map[string]any
			require.NoError(t, json.Unmarshal([]byte(gotBody.Load().(string)), &sent))
			require.Contains(t, sent, "input", "a Responses endpoint must receive a Responses body")
			require.NotContains(t, sent, "messages", "a raw Chat body must not reach a Responses endpoint")
		})
	}
}
