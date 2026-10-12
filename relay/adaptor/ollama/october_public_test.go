package ollama_test

import (
	"github.com/Laisky/one-api/relay/adaptor/ollama"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberOllamaTagMetadata verifies exact curated tag metadata and retained
// symbolic rates using t. It returns nothing and does not contact an Ollama host.
func TestOctoberOllamaTagMetadata(t *testing.T) {
	t.Parallel()
	a := &ollama.Adaptor{}
	cfg, ok := a.GetDefaultModelPricing()["nomic-embed-text:latest"]
	require.True(t, ok)
	require.Equal(t, []string{"embeddings"}, cfg.OutputModalities)
	require.Equal(t, "fp16", cfg.Quantization)
	require.EqualValues(t, 8192, cfg.ContextLength)
	require.Zero(t, cfg.MaxOutputTokens)
	require.InDelta(t, .005, a.GetModelRatio("nomic-embed-text:latest")/ratio.MilliTokensUsd, 1e-12)
	for _, id := range []string{"deepseek-r1:latest", "qwen3:latest"} {
		c := a.GetDefaultModelPricing()[id]
		require.Empty(t, c.SupportedReasoningEfforts)
		require.Empty(t, c.DefaultReasoningEffort)
		require.Contains(t, c.SupportedFeatures, "reasoning")
		require.Contains(t, c.SupportedFeatures, "tools")
		require.Contains(t, c.Description, "native boolean")
		require.InDelta(t, .01, a.GetModelRatio(id)/ratio.MilliTokensUsd, 1e-12)
	}
	gpt := a.GetDefaultModelPricing()["gpt-oss:latest"]
	require.Equal(t, []string{"low", "medium", "high"}, gpt.SupportedReasoningEfforts)
	require.EqualValues(t, 131072, a.GetDefaultModelPricing()["phi3:latest"].ContextLength)
	url, err := a.GetRequestURL(&meta.Meta{Mode: relaymode.Embeddings, BaseURL: "http://localhost:11434"})
	require.NoError(t, err)
	require.Equal(t, "http://localhost:11434/api/embed", url)
	request := &model.GeneralOpenAIRequest{Model: "nomic-embed-text:latest", Input: []any{"first", "second"}}
	converted, err := a.ConvertRequest(nil, relaymode.Embeddings, request)
	require.NoError(t, err)
	out, ok := converted.(*ollama.EmbeddingRequest)
	require.True(t, ok)
	require.Equal(t, request.Model, out.Model)
	require.Equal(t, []string{"first", "second"}, out.Input)
	require.Greater(t, a.GetModelRatio("unknown-local-tag"), 0.0)
}
