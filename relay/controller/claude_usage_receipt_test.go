package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestRelayClaudeMessagesClearsConvertedUsageReceiptPerAttempt verifies a
// retry that reuses the gin context cannot settle on the previous attempt's
// converted-response receipt: the receipt is cleared when an attempt starts,
// like the other per-attempt Claude delivery receipts.
func TestRelayClaudeMessagesClearsConvertedUsageReceiptPerAttempt(t *testing.T) {
	const balance int64 = 10_000_000
	xaiVideoSetup(t, balance, false)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"overloaded","type":"server_error"}}`)
	}))
	t.Cleanup(upstream.Close)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })

	up := claudeUsageUpstreams()[0]
	c, _, _ := protocolContext(t, up.channelType, up.actualModel, "/v1/messages", claudeConformanceRequest(false), upstream.URL, balance, 1, false, &claudeUsagePricing)
	stale := &relaymodel.Usage{PromptTokens: 999_999, CompletionTokens: 1}
	c.Set(ctxkey.ConvertedResponseUsage, stale)
	require.NotNil(t, RelayClaudeMessagesHelper(c), "the upstream failure must surface")
	drainCriticalTasks(t)
	raw, exists := c.Get(ctxkey.ConvertedResponseUsage)
	require.True(t, exists)
	receipt, _ := raw.(*relaymodel.Usage)
	require.Nil(t, receipt, "the previous attempt's receipt must be cleared at attempt start")
}

// TestConvertedResponseBillingUsage verifies the controller reads only a valid
// receipt and settles on a copy, so billing adjustments never mutate the
// adaptor's recorded receipt.
func TestConvertedResponseBillingUsage(t *testing.T) {
	c, _, _ := xaiVideoContext(t, http.MethodPost, "/v1/messages", "{}", "https://example.invalid", 1, 1, false, nil)
	require.Nil(t, convertedResponseBillingUsage(c))
	c.Set(ctxkey.ConvertedResponseUsage, nil)
	require.Nil(t, convertedResponseBillingUsage(c))
	c.Set(ctxkey.ConvertedResponseUsage, "not a receipt")
	require.Nil(t, convertedResponseBillingUsage(c))

	recorded := &relaymodel.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12}
	c.Set(ctxkey.ConvertedResponseUsage, recorded)
	settled := convertedResponseBillingUsage(c)
	require.Equal(t, recorded, settled)
	require.NotSame(t, recorded, settled)
	settled.BillingEstimateReason = "adjusted"
	require.Empty(t, recorded.BillingEstimateReason)
}

// TestConvertedClaudeBodyUsageFallback pins the body-parsing fallback kept for
// converters that record no receipt: Claude JSON counters (OpenAI semantics, as
// those converters emit), the missing-counter and unknown-payload estimates,
// and SSE bodies.
func TestConvertedClaudeBodyUsageFallback(t *testing.T) {
	lg := logger.Logger
	parsed := convertedClaudeBodyUsage(lg, []byte(`{"type":"message","content":[],"usage":{"input_tokens":9,"output_tokens":2,"cache_read_input_tokens":4,"cache_creation_input_tokens":3}}`), "application/json", 50, "gpt-4o")
	require.Equal(t, &relaymodel.Usage{PromptTokens: 9, CompletionTokens: 2, TotalTokens: 11, CacheWrite5mTokens: 3,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 4}}, parsed)

	split := convertedClaudeBodyUsage(lg, []byte(`{"type":"message","content":[],"usage":{"input_tokens":9,"output_tokens":2,"cache_creation_input_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3}}}`), "application/json", 50, "gpt-4o")
	require.Equal(t, 2, split.CacheWrite5mTokens)
	require.Equal(t, 3, split.CacheWrite1hTokens)
	require.Nil(t, split.PromptTokensDetails)

	missing := convertedClaudeBodyUsage(lg, []byte(`{"type":"message","content":[{"type":"text","text":"Hello world"}],"usage":{"input_tokens":0,"output_tokens":0}}`), "application/json", 50, "gpt-4o")
	require.Equal(t, 50, missing.PromptTokens)
	require.Positive(t, missing.CompletionTokens)
	require.Equal(t, "converted_claude_usage_missing_counters", missing.BillingEstimateReason)

	unknown := convertedClaudeBodyUsage(lg, []byte(`not json`), "text/plain", 50, "gpt-4o")
	require.Equal(t, &relaymodel.Usage{PromptTokens: 50, TotalTokens: 50, BillingEstimateReason: "converted_claude_usage_unknown_payload"}, unknown)

	stream := convertedClaudeBodyUsage(lg, []byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":9,\"output_tokens\":2}}\n\n"), "text/event-stream", 50, "gpt-4o")
	require.Equal(t, 9, stream.PromptTokens)
	require.Equal(t, 2, stream.CompletionTokens)
}
