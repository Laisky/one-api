package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const clientCompatibilityClaudeBody = `{"model":"alias","max_tokens":64,"system":[{"type":"text","text":"Cacheable system prefix <native>&","cache_control":{"type":"ephemeral","ttl":"1h"},"future_number":9007199254740993}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"signed","signature":"<opaque>&signature"},{"type":"text","text":"Ready"}]},{"role":"user","content":"Continue"}],"future_protocol_field":{"exact":9007199254740993},"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"key":{"type":"string"}}}}]}`
const clientCompatibilityClaudeJSON = `{"id":"msg_client_compatibility","type":"message","role":"assistant","model":"claude-client-compatibility","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`
const clientCompatibilityClaudeSSE = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_client_compatibility\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-client-compatibility\",\"content\":[],\"usage\":{\"input_tokens\":11,\"output_tokens\":1}}}\n\n" +
	"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

// clientCompatibilityClaudeObservation captures synthetic protocol fields only.
// Parameters: none. Returns: this data-only fixture carries request observations.
type clientCompatibilityClaudeObservation struct {
	Header http.Header
	Body   map[string]json.RawMessage
	Err    error
}

// clientCompatibilityClaudeExchange runs one actual TLS native Messages request.
// Parameters: t owns fixtures and stream selects SSE. Returns: the provider
// observation, downstream recorder, handled context, and any relay failure.
func clientCompatibilityClaudeExchange(t *testing.T, stream bool) (clientCompatibilityClaudeObservation, *httptest.ResponseRecorder, *gin.Context, *relaymodel.ErrorWithStatusCode) {
	t.Helper()
	const balance = int64(100_000)
	xaiVideoSetup(t, balance, false)
	observed := make(chan clientCompatibilityClaudeObservation, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := clientCompatibilityClaudeObservation{Header: r.Header.Clone()}
		got.Err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&got.Body)
		observed <- got
		w.Header().Set("request-id", "req_upstream_native")
		w.Header().Set("retry-after", "2")
		w.Header().Set("anthropic-ratelimit-requests-remaining", "9")
		w.Header().Set("anthropic-ratelimit-tokens-reset", "2026-10-17T00:00:01Z")
		w.Header().Set("Set-Cookie", "upstream-cookie=synthetic")
		w.Header().Set("Authorization", "Bearer synthetic-upstream-response")
		w.Header().Set("X-Upstream-Private", "synthetic-private-header")
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, clientCompatibilityClaudeSSE)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, clientCompatibilityClaudeJSON)
		}
	}))
	t.Cleanup(upstream.Close)
	oldClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = oldClient })
	body := clientCompatibilityClaudeBody
	if stream {
		body = body[:len(body)-1] + `,"stream":true}`
	}
	c, w, id := protocolContext(t, channeltype.Anthropic, "claude-client-compatibility", "/v1/messages", body, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	c.Set(ctxkey.Config, model.ChannelConfig{CustomHeaders: map[string]string{"Authorization": "Bearer configured-upstream-auth", "X-Api-Key": "configured-upstream-key"}})
	c.Request.Header.Set("Authorization", "Bearer caller-gateway-auth")
	c.Request.Header.Set("x-api-key", "caller-gateway-key")
	c.Request.Header.Set("anthropic-version", "2023-06-01")
	c.Request.Header.Set("anthropic-beta", "caller-beta,context-management-2025-06-27")
	c.Request.Header.Set("anthropic-protocol-feature", "future-protocol-v1")
	c.Request.Header.Set("anthropic-workspace-id", "workspace-synthetic-protocol")
	responseNamespaceCleanLedger(t, id)
	apiErr := RelayClaudeMessagesHelper(c)
	drainCriticalTasks(t)
	var got clientCompatibilityClaudeObservation
	select {
	case got = <-observed:
	default:
		t.Fatal("native fixture did not receive provider request")
	}
	require.NoError(t, got.Err)
	return got, w, c, apiErr
}

// TestClientCompatibilityClaudeProtocolHeadersAndNativeBody verifies caller
// protocol headers and opaque system/cache fields survive native dispatch.
// Parameters: t owns JSON/SSE fixtures. Returns: nothing; provider credentials
// remain selected by the channel and cannot be replaced by caller credentials.
func TestClientCompatibilityClaudeProtocolHeadersAndNativeBody(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			got, _, _, apiErr := clientCompatibilityClaudeExchange(t, stream)
			require.Nil(t, apiErr)
			require.Equal(t, "2023-06-01", got.Header.Get("anthropic-version"))
			require.Contains(t, got.Header.Get("anthropic-beta"), "caller-beta")
			require.Contains(t, got.Header.Get("anthropic-beta"), "context-management-2025-06-27")
			require.Equal(t, "Bearer configured-upstream-auth", got.Header.Get("Authorization"))
			require.Equal(t, "configured-upstream-key", got.Header.Get("x-api-key"))
			var original map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(clientCompatibilityClaudeBody), &original))
			for _, field := range []string{"system", "messages", "tools", "future_protocol_field"} {
				require.Equal(t, original[field], got.Body[field], field)
			}
			require.Equal(t, "future-protocol-v1", got.Header.Get("anthropic-protocol-feature"))
			require.Equal(t, "workspace-synthetic-protocol", got.Header.Get("anthropic-workspace-id"))
		})
	}
}

// TestClientCompatibilityClaudeNativeResponseHeaders checks the selected safe
// headers on both native JSON and SSE, retaining JSON success as a control.
// Parameters: t owns fixtures. Returns: nothing; cookies, auth and arbitrary
// provider headers must not leak while correlation and retry metadata survive.
func TestClientCompatibilityClaudeNativeResponseHeaders(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			_, w, _, apiErr := clientCompatibilityClaudeExchange(t, stream)
			require.Nil(t, apiErr)
			require.Equal(t, http.StatusOK, w.Code)
			if stream {
				require.Contains(t, w.Body.String(), "event: message_stop")
			} else {
				require.JSONEq(t, clientCompatibilityClaudeJSON, w.Body.String())
			}
			require.Empty(t, w.Header().Get("Set-Cookie"))
			require.Empty(t, w.Header().Get("Authorization"))
			require.Empty(t, w.Header().Get("X-Upstream-Private"))
			require.Equal(t, "req_upstream_native", w.Header().Get("request-id"))
			require.Equal(t, "2", w.Header().Get("retry-after"))
			require.Equal(t, "9", w.Header().Get("anthropic-ratelimit-requests-remaining"))
			require.Equal(t, "2026-10-17T00:00:01Z", w.Header().Get("anthropic-ratelimit-tokens-reset"))
		})
	}
}
