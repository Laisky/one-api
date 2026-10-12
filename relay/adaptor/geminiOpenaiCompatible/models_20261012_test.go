package geminiOpenaiCompatible_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGeminiOctoberLifecycle checks published migration guidance through both
// shared and native catalogues. It accepts t and returns no values.
func TestGeminiOctoberLifecycle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id, replacement, date  string
		input, cached, output float64
	}{
		{"gemini-3.7-flash", "gemini-3.8-flash", "2026-10-08", 0.75, 0.075, 3.75},
		{"gemini-3.5-flash", "gemini-3.6-flash", "2026-10-08", 1.50, 0.15, 9.00},
		{"gemini-3.1-flash-image", "gemini-nano-banana-2.1", "2026-10-06", 0.50, 0, 3.00},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := geminiOpenaiCompatible.ModelRatios[tc.id]
			require.True(t, exists)
			require.Contains(t, geminiOpenaiCompatible.ModelList, tc.id)
			require.Contains(t, gemini.ModelList, tc.id)
			require.Equal(t, cfg, gemini.ModelRatios[tc.id])
			require.Contains(t, cfg.Description, "Deprecated in the Gemini Developer API")
			require.Contains(t, cfg.Description, tc.date)
			require.Contains(t, cfg.Description, tc.replacement)
			require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
		})
	}
	require.Contains(t, geminiOpenaiCompatible.ModelRatios["gemini-3.1-flash-image"].Description, "no shutdown date announced")
	require.Contains(t, geminiOpenaiCompatible.ModelRatios["gemini-3.5-flash"].Description, "not a verified current tariff")
	for _, id := range []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite"} {
		require.Contains(t, geminiOpenaiCompatible.ModelRatios[id].Description, "not deprecated")
	}
}
