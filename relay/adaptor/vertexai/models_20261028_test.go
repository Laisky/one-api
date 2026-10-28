package vertexai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiOctober28NanoBananaVertexCatalog checks native Vertex discovery and
// the provider-only implicit-cache rate without changing Gemini Developer pricing.
func TestGeminiOctober28NanoBananaVertexCatalog(t *testing.T) {
	t.Parallel()
	const modelName = "gemini-nano-banana-2.1"
	adapter := &Adaptor{}
	require.Contains(t, adapter.GetModelList(), modelName)
	cfg, exists := adapter.GetDefaultModelPricing()[modelName]
	require.True(t, exists)
	require.EqualValues(t, 131_072, cfg.ContextLength)
	require.EqualValues(t, 32_768, cfg.MaxOutputTokens)
	require.InDelta(t, 1.50, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 7.50, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 0.15, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
	require.NotNil(t, cfg.Image)
	require.InDelta(t, 0.0336, cfg.Image.PricePerImageUsd, 1e-12)
	require.Len(t, cfg.Image.SizeMultipliers, 3)
}

// TestGeminiOctober28NanoBananaVertexURLs verifies that its non-numeric model ID
// does not accidentally send Vertex calls to the default us-central1 endpoint.
func TestGeminiOctober28NanoBananaVertexURLs(t *testing.T) {
	t.Parallel()
	const modelName = "gemini-nano-banana-2.1"
	require.True(t, IsRequireGlobalEndpoint(modelName))
	require.True(t, IsRequireGlobalEndpoint("Gemini-Nano-Banana-2.1"), "mixed-case mapped ID must not be routed regionally")
	require.False(t, IsRequireGlobalEndpoint("gemini-2.0-flash"))
	for _, tc := range []struct {
		stream bool
		action string
	}{
		{false, "generateContent"},
		{true, "streamGenerateContent?alt=sse"},
	} {
		requestMeta := &meta.Meta{
			ActualModelName: modelName,
			Mode:            relaymode.ChatCompletions,
			IsStream:        tc.stream,
		}
		requestMeta.Config.VertexAIProjectID = "nano-banana-regression-project"
		url, err := (&Adaptor{}).GetRequestURL(requestMeta)
		require.NoError(t, err)
		require.Equal(t,
			"https://aiplatform.googleapis.com/v1/projects/nano-banana-regression-project/locations/global/publishers/google/models/"+modelName+":"+tc.action,
			url)
	}
}
