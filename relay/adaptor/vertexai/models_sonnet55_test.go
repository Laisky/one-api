package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	vertexaiClaude "github.com/Laisky/one-api/relay/adaptor/vertexai/claude"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
)

// TestClaudeSonnet55VertexPublicCatalog verifies discovery, child selection,
// global prices, and the provider-specific feature profile. It takes a test
// handle and returns nothing; no credentials or paid calls are used.
func TestClaudeSonnet55VertexPublicCatalog(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	const modelID = "claude-sonnet-5-5"
	require.Contains(t, a.GetModelList(), modelID)
	require.Contains(t, vertexaiClaude.ModelList, modelID)
	require.IsType(t, &vertexaiClaude.Adaptor{}, GetAdaptor(modelID))
	config, ok := a.GetDefaultModelPricing()[modelID]
	require.True(t, ok)
	require.InDelta(t, 2*ratio.MilliTokensUsd, a.GetModelRatio(modelID), 1e-12)
	require.InDelta(t, 10*ratio.MilliTokensUsd, config.Ratio*a.GetCompletionRatio(modelID), 1e-12)
	require.InDelta(t, 0.2*ratio.MilliTokensUsd, config.CachedInputRatio, 1e-12)
	require.InDelta(t, 2.5*ratio.MilliTokensUsd, config.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 4*ratio.MilliTokensUsd, config.CacheWrite1hRatio, 1e-12)
	require.EqualValues(t, 1000000, config.ContextLength)
	require.EqualValues(t, 128000, config.MaxOutputTokens)
	require.Zero(t, config.MaxReasoningTokens)
	require.ElementsMatch(t, []string{"text", "image", "file"}, config.InputModalities)
	require.Equal(t, []string{"text"}, config.OutputModalities)
	require.ElementsMatch(t, []string{"stop", "max_tokens"}, config.SupportedSamplingParameters)
	require.ElementsMatch(t, []string{"tools", "reasoning"}, config.SupportedFeatures)
	require.Empty(t, config.TimeWindows)
	for _, invented := range []string{
		"claude-sonnet-5-5@20260928", "claude-sonnet-5-5-20260928",
		"claude-sonnet-5-5-latest", "anthropic.claude-sonnet-5-5",
	} {
		require.NotContains(t, a.GetModelList(), invented)
	}
	// A conservative Vertex profile must not change first-party metadata.
	require.Contains(t, anthropic.ModelRatios[modelID].SupportedFeatures, "web_search")
	previous := a.GetDefaultModelPricing()["claude-opus-5-5"]
	require.InDelta(t, 4*ratio.MilliTokensUsd, previous.Ratio, 1e-12)
	require.InDelta(t, 0.2*ratio.MilliTokensUsd, previous.CachedInputRatio, 1e-12)
}

// TestClaudeSonnet55VertexRequestURL checks the published undated publisher ID
// on the global endpoint. It takes a test handle and returns nothing.
func TestClaudeSonnet55VertexRequestURL(t *testing.T) {
	t.Parallel()
	requestMeta := &meta.Meta{
		ActualModelName: "claude-sonnet-5-5",
		BaseURL:         "https://aiplatform.googleapis.com",
	}
	requestMeta.Config.VertexAIProjectID = "catalog-test-project"
	requestMeta.Config.Region = "global"
	url, err := (&Adaptor{}).GetRequestURL(requestMeta)
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/catalog-test-project/locations/global/publishers/anthropic/models/claude-sonnet-5-5:rawPredict", url)
}
