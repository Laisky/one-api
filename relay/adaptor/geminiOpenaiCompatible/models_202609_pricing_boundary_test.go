package geminiOpenaiCompatible

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGeminiSeptember2026FlashPricingBoundary verifies the production pricing
// resolver against Google's fixed published values immediately before and at
// the 2027 UTC transition for every affected model. Parameters: t is the current
// test handle. Returns: none.
func TestGeminiSeptember2026FlashPricingBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		effectiveAt     time.Time
		inputUsd        float64
		outputUsd       float64
		cachedInputUsd  float64
		completionRatio float64
	}{
		{
			name:            "promotional-last-instant",
			effectiveAt:     time.Date(2026, time.December, 31, 23, 59, 59, 999_999_999, time.UTC),
			inputUsd:        0.75,
			outputUsd:       3.75,
			cachedInputUsd:  0.075,
			completionRatio: 5,
		},
		{
			name:            "standard-first-instant",
			effectiveAt:     time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
			inputUsd:        1.50,
			outputUsd:       7.50,
			cachedInputUsd:  0.15,
			completionRatio: 5,
		},
	}

	for _, model := range []string{"gemini-3.6-flash", "gemini-3.7-flash", "gemini-3.8-flash"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()

			config, ok := ModelRatios[model]
			require.True(t, ok, "%s missing from pricing map", model)
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					resolved := pricing.ApplyTimeWindow(config, tt.effectiveAt)
					require.InDelta(t, tt.inputUsd*ratio.MilliTokensUsd, resolved.Ratio, 1e-12)
					require.InDelta(t, tt.completionRatio, resolved.CompletionRatio, 1e-12)
					require.InDelta(t, tt.outputUsd*ratio.MilliTokensUsd, resolved.Ratio*resolved.CompletionRatio, 1e-12)
					require.InDelta(t, tt.cachedInputUsd*ratio.MilliTokensUsd, resolved.CachedInputRatio, 1e-12)
				})
			}
		})
	}
}
