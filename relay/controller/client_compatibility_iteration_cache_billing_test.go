package controller

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestClientCompatibilityIterationRootCacheHTTPBilling verifies real SSE loopback
// settlement adds compaction cache buckets once to updated cumulative message caches.
func TestClientCompatibilityIterationRootCacheHTTPBilling(t *testing.T) {
	const balance = int64(1_000_000)
	const initial = `{"input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6},"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276,"cache_read_input_tokens":5,"cache_creation_input_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3}},{"type":"message","input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6}}]}`
	const final = `{"output_tokens":7,"cache_read_input_tokens":31,"cache_creation_input_tokens":18,"cache_creation":{"ephemeral_5m_input_tokens":7,"ephemeral_1h_input_tokens":11}}`
	body := clientCompatibilityCompactionResponse(t, true, initial)
	body = strings.ReplaceAll(body, `"delta":{"stop_reason":"compaction"},"usage":`+initial, `"delta":{"stop_reason":"compaction"},"usage":`+final)
	result := runClaudeToolPolicyCase(t, balance, claudeToolPolicyCase{
		channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: "text/event-stream", response: body, payload: claudeToolPolicyPayload(t, true, nil, nil), beta: "compact-2026-09-04",
		prepare: func(c *gin.Context) {
			responseNamespaceCleanLedger(t, c.GetString(ctxkey.RequestId))
			require.NoError(t, c.MustGet(ctxkey.ChannelModel).(*model.Channel).SetModelPriceConfigs(map[string]model.ModelConfigLocal{"claude-sonnet-4": {Ratio: 1, CompletionRatio: 2, CachedInputRatio: 3, CacheWrite5mRatio: 4, CacheWrite1hRatio: 5}}))
		},
	})
	require.Nil(t, result.apiErr)
	require.EqualValues(t, 1, result.calls)
	// 155 input +283*2 output +(5+31)*3 reads +(2+7)*4 five-minute +(3+11)*5 one-hour.
	requireClaudeToolPolicyLedger(t, result, balance, 935)
}
