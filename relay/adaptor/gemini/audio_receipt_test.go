package gemini

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// TestReview607AudioReceipt preserves public totals and private billing evidence.
func TestReview607AudioReceipt(t *testing.T) {
	for _, known := range []bool{false, true} {
		m := &UsageMetadata{PromptTokenCount: 100, CachedContentTokenCount: 30, CandidatesTokenCount: 2, TotalTokenCount: 102, PromptTokensDetails: []PromptTokensDetails{{Modality: "AUDIO", TokenCount: 80}, {Modality: "TEXT", TokenCount: 20}}}
		if known {
			m.CacheTokensDetails = []PromptTokensDetails{{Modality: "AUDIO", TokenCount: 20}, {Modality: "TEXT", TokenCount: 10}}
		}
		u := geminiUsageMetadataToOpenAIUsage(m)
		require.Equal(t, 100, u.PromptTokens)
		require.Equal(t, 102, u.TotalTokens)
		require.Equal(t, 80, u.PromptTokensDetails.AudioTokens)
		require.Equal(t, 30, u.PromptTokensDetails.CachedTokens)
		require.NotNil(t, u.NativeGoogleAudio)
		require.Equal(t, known, u.NativeGoogleAudio.CachedKnown)
	}
}
