package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestGPT61TerraCacheWriteUsageRoundTrip verifies that an OpenAI response
// converted through the shared usage representation does not lose TTL detail.
func TestGPT61TerraCacheWriteUsageRoundTrip(t *testing.T) {
	raw := &ResponseAPIUsage{
		InputTokens:        8000,
		OutputTokens:       10,
		InputTokensDetails: &ResponseAPIInputTokensDetails{
			CacheWriteTokens:        7000,
			CacheWriteTokensDetails: &model.UsageCacheWriteTokensDetails{
				TTL30mTokens: 2000,
				TTL12hTokens: 5000,
			},
		},
	}
	canonical := raw.ToModelUsage()
	require.NotNil(t, canonical)

	reencoded := (&ResponseAPIUsage{}).FromModelUsage(canonical)
	require.NotNil(t, reencoded.InputTokensDetails)
	require.NotNil(t, reencoded.InputTokensDetails.CacheWriteTokensDetails)
	require.Equal(t, 2000, reencoded.InputTokensDetails.CacheWriteTokensDetails.TTL30mTokens)
	require.Equal(t, 5000, reencoded.InputTokensDetails.CacheWriteTokensDetails.TTL12hTokens)
	require.Equal(t, 7000, reencoded.InputTokensDetails.CacheWriteTokens)
}
