package jina

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestModelCatalog verifies native IDs, researched prices and non-generative limits.
func TestModelCatalog(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	require.Equal(t, "jina", a.GetChannelName())
	require.Len(t, a.GetModelList(), 25)
	for name, cfg := range a.GetDefaultModelPricing() {
		require.Contains(t, a.GetModelList(), name)
		require.Positive(t, cfg.ContextLength, name)
		require.NotEmpty(t, cfg.Description, name)
		require.NotEmpty(t, cfg.InputModalities, name)
		require.Nil(t, cfg.PerCall, "search must not become per-call billing: %s", name)
		usd := 0.05
		if name == "jina-embeddings-v5-text-nano" || name == "jina-embeddings-v5-omni-nano" {
			usd = 0.02
		}
		if name == "jina-ocr-v1" {
			usd = 0.50
			require.Equal(t, 4.0, a.GetCompletionRatio(name))
			require.Equal(t, int32(8192), cfg.MaxOutputTokens)
		} else {
			require.Zero(t, cfg.MaxOutputTokens, "embedding width is not an output-token limit: %s", name)
		}
		require.InDelta(t, usd*ratio.MilliTokensUsd, a.GetModelRatio(name), 1e-12, name)
	}
	for _, unavailable := range []string{"readerlm-v2", "jina-reader-lm", "jina-vlm", "jina-embedding-b-en-v1"} {
		require.NotContains(t, a.GetModelList(), unavailable)
	}
	require.Equal(t, int32(134144), ModelRatios["jina-reranker-v3"].ContextLength)
	require.Contains(t, ModelRatios["jina-reranker-m0"].InputModalities, "image")
	require.Contains(t, ModelRatios["jina-embeddings-v5-omni-small"].InputModalities, "audio")
	require.InDelta(t, 2.5*ratio.MilliTokensUsd, a.GetModelRatio("custom-model"), 1e-12)
}
