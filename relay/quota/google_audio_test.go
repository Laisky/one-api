package quota

import (
	"testing"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestReview607NativeAudioSurcharge qualifies measured ordinary audio pricing
// while protecting other providers and uncertainty on cached input.
func TestReview607NativeAudioSurcharge(t *testing.T) {
	for _, tc := range []struct {
		name      string
		receipt   *model.NativeGoogleAudioUsage
		cached    int
		want      float64
		uncertain bool
	}{
		{"text-unchanged", nil, 0, 0, false},
		{"audio", &model.NativeGoogleAudioUsage{Tokens: 80, CachedKnown: true}, 0, 160, false},
		{"cached-audio", &model.NativeGoogleAudioUsage{Tokens: 80, CachedTokens: 20, CachedKnown: true}, 30, 124, false},
		{"missing-cache-details", &model.NativeGoogleAudioUsage{Tokens: 80}, 30, 0, true},
		{"invalid", &model.NativeGoogleAudioUsage{Tokens: 101, CachedKnown: true}, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := &model.Usage{PromptTokens: 100, NativeGoogleAudio: tc.receipt}
			cost, uncertain := nativeGoogleAudioSurcharge(u, adaptor.ModelConfig{Audio: &adaptor.AudioPricingConfig{PromptRatio: 3}}, 1, 0.1, tc.cached)
			require.InDelta(t, tc.want, cost, 1e-9)
			require.Equal(t, tc.uncertain, uncertain)
		})
	}
}
