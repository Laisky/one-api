package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestGPTImage25PromptCharacterBoundary reproduces byte-count rejection of valid
// Unicode prompts. Parameters: t runs catalog and fallback cases. Returns: none.
func TestGPTImage25PromptCharacterBoundary(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-image-2.5-sunburst", "gpt-image-2.5-sunburst-2026-09-08", "gpt-image-2.5-flare", "gpt-image-2.5-flare-2026-09-08"} {
		for _, character := range []string{"a", "界", "🌈"} {
			req := &relaymodel.ImageRequest{Model: name, Prompt: strings.Repeat(character, 32000)}
			require.True(t, isValidImagePromptLength(req, openai.ModelRatios[name].Image), "32000 code points must fit: %s", name)
			req.Prompt += character
			require.False(t, isValidImagePromptLength(req, openai.ModelRatios[name].Image), "32001 code points must not fit: %s", name)
		}
	}
}
