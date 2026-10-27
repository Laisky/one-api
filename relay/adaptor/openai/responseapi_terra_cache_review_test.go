package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGPT61TerraResponseUsageThirtyMinuteCacheWrites verifies that an actual
// OpenAI Responses usage receipt charges nested 30-minute writes rather than
// treating the entire input as uncached, and does not count legacy fields twice.
func TestGPT61TerraResponseUsageThirtyMinuteCacheWrites(t *testing.T) {
	for _, tc := range []struct {
		name string
		legacy int
	}{
		{name: "nested_only"},
		{name: "nested_and_legacy", legacy: 7_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"input_tokens":12000,"output_tokens":100,"input_tokens_details":{"cached_tokens":4000,"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":7000,"ttl_12h_tokens":0}}}`
			if tc.legacy != 0 {
				raw = `{"input_tokens":12000,"output_tokens":100,"cache_write_tokens":7000,"input_tokens_details":{"cached_tokens":4000,"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":7000,"ttl_12h_tokens":0}}}`
			}
			var decoded ResponseAPIUsage
			require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
			usage := decoded.ToModelUsage()
			require.NotNil(t, usage)
			require.Equal(t, 12_000, usage.PromptTokens)
			require.Equal(t, 4_000, usage.PromptTokensDetails.CachedTokens)
			require.Equal(t, 7_000, usage.CacheWrite5mTokens)
		})
	}
}
