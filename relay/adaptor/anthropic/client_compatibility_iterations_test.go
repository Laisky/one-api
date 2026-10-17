package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientCompatibilityIterationReceipts checks official canonical iteration accounting.
// The fixtures follow compaction-on-demand and compaction-threshold usage examples;
// top-level counters exclude compaction, while iterations already include message work.
func TestClientCompatibilityIterationReceipts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, raw                               string
		input, output, cached, write5m, write1h int
	}{
		{name: "on_demand", raw: `{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276}]}`, input: 144, output: 276},
		{name: "threshold_canonical_not_top_plus_iterations", raw: `{"input_tokens":23000,"output_tokens":1000,"iterations":[{"type":"compaction","input_tokens":180000,"output_tokens":3500},{"type":"message","input_tokens":23000,"output_tokens":1000}]}`, input: 203000, output: 4500},
		{name: "beta_message_without_compaction", raw: `{"input_tokens":11,"output_tokens":7,"iterations":[{"type":"message","input_tokens":11,"output_tokens":7}]}`, input: 11, output: 7},
		{name: "ordinary_without_beta", raw: `{"input_tokens":11,"output_tokens":7}`, input: 11, output: 7},
		{name: "iteration_cache_ttl_buckets", raw: `{"input_tokens":5,"output_tokens":7,"cache_read_input_tokens":20,"cache_creation_input_tokens":11,"cache_creation":{"ephemeral_5m_input_tokens":5,"ephemeral_1h_input_tokens":6},"iterations":[{"type":"compaction","input_tokens":3,"output_tokens":4,"cache_read_input_tokens":10,"cache_creation_input_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":5}},{"type":"message","input_tokens":5,"output_tokens":7,"cache_read_input_tokens":20,"cache_creation_input_tokens":11,"cache_creation":{"ephemeral_5m_input_tokens":5,"ephemeral_1h_input_tokens":6}}]}`, input: 8, output: 11, cached: 30, write5m: 7, write1h: 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var receipt httpUsageReceipt
			require.NoError(t, receipt.apply(json.RawMessage(tc.raw)))
			usage := receipt.snapshot()
			require.NotNil(t, usage)
			require.Equal(t, tc.input, usage.PromptTokens)
			require.Equal(t, tc.output, usage.CompletionTokens)
			require.Equal(t, tc.input+tc.output, usage.TotalTokens)
			cached := 0
			if usage.PromptTokensDetails != nil {
				cached = usage.PromptTokensDetails.CachedTokens
			}
			require.Equal(t, tc.cached, cached)
			require.Equal(t, tc.write5m, usage.CacheWrite5mTokens)
			require.Equal(t, tc.write1h, usage.CacheWrite1hTokens)
		})
	}
}

// TestClientCompatibilityIterationCumulativeReplacement verifies repeated SSE receipts
// replace their iteration snapshot instead of adding the same compaction work twice.
func TestClientCompatibilityIterationCumulativeReplacement(t *testing.T) {
	t.Parallel()
	var receipt httpUsageReceipt
	updates := []string{
		`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":100}]}`,
		`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276}]}`,
		`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276}]}`,
	}
	for _, raw := range updates {
		require.NoError(t, receipt.apply(json.RawMessage(raw)))
	}
	usage := receipt.snapshot()
	require.Equal(t, 144, usage.PromptTokens)
	require.Equal(t, 276, usage.CompletionTokens)
	require.Equal(t, 420, usage.TotalTokens)
}

// TestClientCompatibilityInvalidIterationIsAtomic requires malformed iteration counts
// to preserve the previous verified receipt, including existing cache TTL invariants.
func TestClientCompatibilityInvalidIterationIsAtomic(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":-1,"output_tokens":7}]}`,
		`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":1.5,"output_tokens":7}]}`,
		`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":1,"output_tokens":7,"cache_creation_input_tokens":9,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3}}]}`,
		`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":9223372036854775807,"output_tokens":1}]}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var receipt httpUsageReceipt
			require.NoError(t, receipt.apply(json.RawMessage(`{"input_tokens":11,"output_tokens":7}`)))
			before := receipt.snapshot()
			require.Error(t, receipt.apply(json.RawMessage(raw)))
			require.Equal(t, before, receipt.snapshot())
		})
	}
}

// TestClientCompatibilityIterationPartialFields retains earlier indexed counters
// and one-hour cache writes when later cumulative snapshots omit their split.
// Root-only metadata must not replace canonical totals with non-compaction totals.
func TestClientCompatibilityIterationPartialFields(t *testing.T) {
	t.Parallel()
	var receipt httpUsageReceipt
	updates := []string{
		`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276,"cache_creation_input_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":5}}]}`,
		`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","cache_creation_input_tokens":7}]}`,
		`{"service_tier":"standard"}`,
	}
	for _, raw := range updates {
		require.NoError(t, receipt.apply(json.RawMessage(raw)))
	}
	usage := receipt.snapshot()
	require.Equal(t, 144, usage.PromptTokens)
	require.Equal(t, 276, usage.CompletionTokens)
	require.Equal(t, 2, usage.CacheWrite5mTokens)
	require.Equal(t, 5, usage.CacheWrite1hTokens)
	require.Equal(t, "standard", usage.ServiceTier)
	before := receipt.snapshot()
	require.Error(t, receipt.apply(json.RawMessage(`{"iterations":[{"type":"compaction","cache_creation_input_tokens":8}]}`)))
	require.Equal(t, before, receipt.snapshot())
}

// TestClientCompatibilityIterationCrossEntryOverflow checks sums across separate
// valid entries and preserves the last verified receipt when their sum overflows.
func TestClientCompatibilityIterationCrossEntryOverflow(t *testing.T) {
	t.Parallel()
	var receipt httpUsageReceipt
	require.NoError(t, receipt.apply(json.RawMessage(`{"input_tokens":11,"output_tokens":7}`)))
	before := receipt.snapshot()
	require.Error(t, receipt.apply(json.RawMessage(`{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":9223372036854775807,"output_tokens":0},{"type":"message","input_tokens":1,"output_tokens":0}]}`)))
	require.Equal(t, before, receipt.snapshot())
}
