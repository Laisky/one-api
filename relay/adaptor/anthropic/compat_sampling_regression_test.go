package anthropic

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestClaudeMythosPreviewSamplingCompatibility covers the published sampling
// restriction without assuming that sampling support determines thinking mode.
func TestClaudeMythosPreviewSamplingCompatibility(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"claude-mythos-preview", " CLAUDE-MYTHOS-PREVIEW ", "claude-mythos-preview-20260407"} {
		t.Run(name, func(t *testing.T) {
			temperature, topP, topK := 0.0, 0.8, 10
			tempPtr, topPPtr, topKPtr := &temperature, &topP, &topK
			thinking := &model.Thinking{Type: "disabled"}
			NormalizeModelCompatibility(name, &tempPtr, &topPPtr, &topKPtr, &thinking)
			require.Nil(t, tempPtr)
			require.Nil(t, topPPtr)
			require.Nil(t, topKPtr)
			require.Equal(t, "disabled", thinking.Type, "sampling cleanup must not enable thinking")
			NormalizeModelCompatibility(name, &tempPtr, &topPPtr, &topKPtr, &thinking)
			require.Equal(t, "disabled", thinking.Type)
			NormalizeModelCompatibility(name, nil, nil, nil, nil)
		})
	}
}

// TestClaudeSamplingCompatibilityKeepsLegacyControls prevents the new rule
// from stripping supported sampling on older models and custom lookalikes.
func TestClaudeSamplingCompatibilityKeepsLegacyControls(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"claude-sonnet-4-6", "claude-mythos-previewish", "custom-model"} {
		t.Run(name, func(t *testing.T) {
			temperature, topK := 0.0, 10
			tempPtr, topKPtr := &temperature, &topK
			var topP *float64
			NormalizeModelCompatibility(name, &tempPtr, &topP, &topKPtr, nil)
			require.Same(t, &temperature, tempPtr)
			require.Same(t, &topK, topKPtr)
		})
	}
}
