package gemini

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
)

// TestGeminiSeptember30SharedCatalog checks dependency initialization and preserves
// the distinction between a TTS catalog entry and native REST system instructions.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember30SharedCatalog(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"gemini-3.8-flash-tts", "gemini-3.8-flash-lite-tts"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, ModelList, model)
			config, ok := ModelRatios[model]
			require.True(t, ok)
			require.Equal(t, geminiOpenaiCompatible.ModelRatios[model], config)
			require.False(t, IsModelSupportSystemInstruction(model))
			require.Empty(t, config.SupportedReasoningEfforts)
			require.Equal(t, []string{"audio"}, config.OutputModalities)
		})
	}
}
