package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientCompatibilityIterationRootCacheDeltas checks the SDK's cumulative
// non-compaction root cache statistics plus separately billed compaction caches.
// Contract pinned to anthropic-sdk-python bcd55fe1d3f0f1240765f8cef04295b67337f55a:
// beta_message_delta_usage.py11-15,27-42 and beta_compaction_iteration_usage.py12-18.
func TestClientCompatibilityIterationRootCacheDeltas(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, initial                               string
		compactionCache, compaction5m, compaction1h int
	}{
		{name: "message_only", initial: `{"input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6},"iterations":[{"type":"message","input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6}}]}`},
		{name: "compaction_and_message", initial: `{"input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6},"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276,"cache_read_input_tokens":5,"cache_creation_input_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3}},{"type":"message","input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6}}]}`, compactionCache: 5, compaction5m: 2, compaction1h: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var receipt httpUsageReceipt
			require.NoError(t, receipt.apply(json.RawMessage(tc.initial)))
			for _, raw := range []string{
				`{"output_tokens":7,"cache_read_input_tokens":31,"cache_creation_input_tokens":18,"cache_creation":{"ephemeral_5m_input_tokens":7,"ephemeral_1h_input_tokens":11}}`,
				`{"cache_read_input_tokens":31,"cache_creation_input_tokens":18}`,
				`{"cache_read_input_tokens":31,"cache_creation_input_tokens":18}`,
			} {
				require.NoError(t, receipt.apply(json.RawMessage(raw)))
				usage := receipt.snapshot()
				require.Equal(t, 31+tc.compactionCache, usage.PromptTokensDetails.CachedTokens)
				require.Equal(t, 7+tc.compaction5m, usage.CacheWrite5mTokens)
				require.Equal(t, 11+tc.compaction1h, usage.CacheWrite1hTokens)
				require.Empty(t, usage.BillingEstimateReason)
			}
			before := receipt.snapshot()
			require.Error(t, receipt.apply(json.RawMessage(`{"cache_creation_input_tokens":19}`)))
			require.Equal(t, before, receipt.snapshot(), "inconsistent aggregate cannot erase a verified TTL split")
		})
	}
}
