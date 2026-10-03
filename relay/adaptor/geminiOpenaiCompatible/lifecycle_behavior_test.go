package geminiOpenaiCompatible_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
)

// TestPublishedGeminiShutdownBehavior checks the final catalog consumed by both
// native Gemini and OpenAI-compatible discovery, rather than a raw override map.
// Parameters: t is the test handle. Returns: none.
func TestPublishedGeminiShutdownBehavior(t *testing.T) {
	t.Parallel()
	for catalogName, catalog := range map[string]map[string]adaptor.ModelConfig{
		"native":            (&gemini.Adaptor{}).GetDefaultModelPricing(),
		"openai-compatible": geminiOpenaiCompatible.ModelRatios,
	} {
		t.Run(catalogName, func(t *testing.T) {
			for _, tc := range []struct{ model, date, replacement string }{
				{"gemini-3-pro-preview", "March 9, 2026", "gemini-3.1-pro-preview"},
				{"gemini-3.1-flash-lite-preview", "May 25, 2026", "gemini-3.1-flash-lite"},
				{"gemini-2.5-flash-image-preview", "January 15, 2026", "gemini-3.1-flash-image"},
			} {
				t.Run(tc.model, func(t *testing.T) {
					cfg, ok := catalog[tc.model]
					require.True(t, ok, "retain historical model IDs")
					t.Logf("published description: %s", cfg.Description)
					require.Contains(t, cfg.Description, "was shut down")
					require.Contains(t, cfg.Description, tc.date)
					require.Contains(t, cfg.Description, tc.replacement)
					require.NotContains(t, cfg.Description, "earliest shutdown")
					require.Positive(t, cfg.Ratio, "retain historical billing metadata")
					if tc.model == "gemini-3-pro-preview" {
						require.Contains(t, cfg.Description, "identifier now points to gemini-3.1-pro-preview")
					}
				})
			}
			// A future earliest date is not evidence that a model is already off.
			future := catalog["gemini-2.5-flash-image"].Description
			require.Contains(t, future, "earliest shutdown October 2, 2026")
			require.NotContains(t, future, "was shut down")
		})
	}
}
