package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	vertexaiClaude "github.com/Laisky/one-api/relay/adaptor/vertexai/claude"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
)

// TestClaudeOpus55VertexPublicCatalog verifies the parent channel's advertised
// model, subadapter selection, metadata, and rates. It accepts a test handle and
// returns nothing; no cloud credentials are required.
func TestClaudeOpus55VertexPublicCatalog(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	const modelID = "claude-opus-5-5"
	require.Contains(t, a.GetModelList(), modelID)
	require.Contains(t, vertexaiClaude.ModelList, modelID)
	require.IsType(t, &vertexaiClaude.Adaptor{}, GetAdaptor(modelID))
	config, ok := a.GetDefaultModelPricing()[modelID]
	require.True(t, ok)
	require.InDelta(t, 4*ratio.MilliTokensUsd, a.GetModelRatio(modelID), 1e-12)
	require.InDelta(t, 20*ratio.MilliTokensUsd, config.Ratio*a.GetCompletionRatio(modelID), 1e-12)
	require.InDelta(t, 0.2*ratio.MilliTokensUsd, config.CachedInputRatio, 1e-12)
	require.InDelta(t, 5*ratio.MilliTokensUsd, config.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 8*ratio.MilliTokensUsd, config.CacheWrite1hRatio, 1e-12)
	require.EqualValues(t, 1000000, config.ContextLength)
	require.EqualValues(t, 128000, config.MaxOutputTokens)
	require.Zero(t, config.MaxReasoningTokens)
	require.ElementsMatch(t, []string{"text", "image", "file"}, config.InputModalities)
	require.Equal(t, []string{"text"}, config.OutputModalities)
	require.ElementsMatch(t, []string{"stop", "max_tokens"}, config.SupportedSamplingParameters)
	require.ElementsMatch(t, []string{"tools", "reasoning"}, config.SupportedFeatures)
	require.Empty(t, config.TimeWindows)
	require.NotContains(t, a.GetModelList(), "claude-opus-5-5@20260922")

	// The provider-specific capability profile must not mutate the native one.
	require.Contains(t, anthropic.ModelRatios[modelID].SupportedFeatures, "web_search")
	previous := a.GetDefaultModelPricing()["claude-opus-5"]
	require.InDelta(t, 5*ratio.MilliTokensUsd, previous.Ratio, 1e-12)
	require.InDelta(t, 0.5*ratio.MilliTokensUsd, previous.CachedInputRatio, 1e-12)
}

// TestClaudeOpus55VertexRequestURL verifies that the actual URL uses the
// documented undated publisher ID and honors an explicit global endpoint.
// It accepts a test handle and returns nothing.
func TestClaudeOpus55VertexRequestURL(t *testing.T) {
	t.Parallel()

	requestMeta := &meta.Meta{
		ActualModelName: "claude-opus-5-5",
		BaseURL:         "https://aiplatform.googleapis.com",
	}
	requestMeta.Config.VertexAIProjectID = "catalog-test-project"
	requestMeta.Config.Region = "global"
	url, err := (&Adaptor{}).GetRequestURL(requestMeta)
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/catalog-test-project/locations/global/publishers/anthropic/models/claude-opus-5-5:rawPredict", url)
}
