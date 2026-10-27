package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseUsageTerraThirtyMinuteCacheWrites checks direct and streaming
// Responses usage snapshots with nested cache-write details, ensuring the
// existing settlement buckets receive the actual provider usage.
func TestResponseUsageTerraThirtyMinuteCacheWrites(t *testing.T) {
	body := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":12000,"output_tokens":100,"input_tokens_details":{"cached_tokens":4000,"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":7000,"ttl_12h_tokens":0}}}}}`)
	usage := ParseResponseUsage(body, 0, nil)
	require.NotNil(t, usage)
	require.Equal(t, 12_000, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 4_000, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 7_000, usage.CacheWrite5mTokens)
}
