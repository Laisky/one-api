package gemini

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGeminiSeptember2026NativeCatalog verifies that the native Gemini adaptor
// exposes the new catalog entries and recognizes system-instruction-capable chat models.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember2026NativeCatalog(t *testing.T) {
	t.Parallel()

	for _, model := range []string{
		"gemini-3.8-flash",
		"gemini-3.7-flash",
		"gemini-3.6-flash",
		"gemini-3.5-transcribe",
		"gemini-3.5-transcribe-live",
		"gemini-omni-1.1-flash",
		"gemini-robotics-er-2-preview",
		"gemini-robotics-er-2-streaming-preview",
	} {
		require.Contains(t, ModelList, model)
	}

	for _, model := range []string{
		"gemini-3.8-flash",
		"gemini-3.7-flash",
		"gemini-3.6-flash",
		"gemini-3.5-flash-lite",
	} {
		require.True(t, IsModelSupportSystemInstruction(model), model)
	}
}
