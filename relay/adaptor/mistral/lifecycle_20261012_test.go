package mistral_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/mistral"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestMistralOctoberLifecycle verifies public lifecycle metadata and unchanged
// compatibility pricing using t. It returns no values.
func TestMistralOctoberLifecycle(t *testing.T) {
	t.Parallel()
	a := &mistral.Adaptor{}
	for _, tc := range []struct {
		id, status, date      string
		input, cached, output float64
	}{
		{"zai-glm-5-2", "Deprecated", "2026-10-31", 1.4, 0.14, 4.4},
		{"zai-glm-5-3", "Generally Available", "2026-09-28", 1.4, 0.14, 4.4},
		{"labs-leanstral-1-5", "Retired", "2026-09-30", 0, 0, 0},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := a.GetDefaultModelPricing()[tc.id]
			require.True(t, exists, "retain historical metadata for existing configurations")
			require.Contains(t, cfg.Description, tc.status)
			require.Contains(t, cfg.Description, tc.date)
			require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
		})
	}
}
