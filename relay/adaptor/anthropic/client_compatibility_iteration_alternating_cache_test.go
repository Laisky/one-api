package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientCompatibilityIterationAlternatingRootIndexedCache preserves newer
// cumulative root cache receipts when a later indexed update omits those fields.
// Explicit indexed cache counters still replace the message component exactly;
// compaction remains separate and inconsistent TTL evidence is atomic.
func TestClientCompatibilityIterationAlternatingRootIndexedCache(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, initial, omitted, explicit            string
		compactionCache, compaction5m, compaction1h int
	}{
		{
			name:     "message_only",
			initial:  `{"input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6},"iterations":[{"type":"message","input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6}}]}`,
			omitted:  `{"iterations":[{"type":"message","output_tokens":9}]}`,
			explicit: `{"iterations":[{"type":"message","output_tokens":11,"cache_read_input_tokens":41,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":13,"ephemeral_1h_input_tokens":17}}]}`,
		},
		{
			name:            "compaction_and_message",
			initial:         `{"input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6},"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276,"cache_read_input_tokens":5,"cache_creation_input_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3}},{"type":"message","input_tokens":11,"output_tokens":1,"cache_read_input_tokens":13,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":6}}]}`,
			omitted:         `{"iterations":[{"type":"compaction"},{"type":"message","output_tokens":9}]}`,
			explicit:        `{"iterations":[{"type":"compaction"},{"type":"message","output_tokens":11,"cache_read_input_tokens":41,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":13,"ephemeral_1h_input_tokens":17}}]}`,
			compactionCache: 5, compaction5m: 2, compaction1h: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var receipt httpUsageReceipt
			assertCache := func(cached, write5m, write1h int) {
				t.Helper()
				usage := receipt.snapshot()
				require.NotNil(t, usage)
				require.NotNil(t, usage.PromptTokensDetails)
				require.Equal(t, cached+tc.compactionCache, usage.PromptTokensDetails.CachedTokens)
				require.Equal(t, write5m+tc.compaction5m, usage.CacheWrite5mTokens)
				require.Equal(t, write1h+tc.compaction1h, usage.CacheWrite1hTokens)
				require.Empty(t, usage.BillingEstimateReason)
			}
			require.NoError(t, receipt.apply(json.RawMessage(tc.initial)))
			require.NoError(t, receipt.apply(json.RawMessage(`{"output_tokens":7,"cache_read_input_tokens":31,"cache_creation_input_tokens":18,"cache_creation":{"ephemeral_5m_input_tokens":7,"ephemeral_1h_input_tokens":11}}`)))
			assertCache(31, 7, 11)
			for repeat := 0; repeat < 2; repeat++ {
				require.NoError(t, receipt.apply(json.RawMessage(tc.omitted)))
				assertCache(31, 7, 11)
			}
			before := receipt.snapshot()
			require.Error(t, receipt.apply(json.RawMessage(`{"cache_creation_input_tokens":19}`)))
			require.Equal(t, before, receipt.snapshot(), "contradictory root aggregate must be atomic")
			require.NoError(t, receipt.apply(json.RawMessage(tc.explicit)))
			assertCache(41, 13, 17)
			require.NoError(t, receipt.apply(json.RawMessage(`{"cache_read_input_tokens":41,"cache_creation_input_tokens":30}`)))
			assertCache(41, 13, 17)
			before = receipt.snapshot()
			require.Error(t, receipt.apply(json.RawMessage(`{"cache_creation_input_tokens":31}`)))
			require.Equal(t, before, receipt.snapshot(), "explicit indexed TTL split remains authoritative")
		})
	}
}
