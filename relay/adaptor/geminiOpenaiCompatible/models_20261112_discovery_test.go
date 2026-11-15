package geminiOpenaiCompatible_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiNovember12Discovery verifies public adaptor consumers receive both
// new models after package initialization. Parameter t is the test handle; it returns nothing.
func TestGeminiNovember12Discovery(t *testing.T) {
	t.Parallel()
	native := &gemini.Adaptor{}
	compatible := &openai.Adaptor{ChannelType: channeltype.GeminiOpenAICompatible}
	vertex := &vertexai.Adaptor{}
	for _, provider := range []struct {
		name    string
		models  []string
		pricing map[string]adaptor.ModelConfig
	}{
		{"native", native.GetModelList(), native.GetDefaultModelPricing()},
		{"openai-compatible", compatible.GetModelList(), compatible.GetDefaultModelPricing()},
		{"vertex", vertex.GetModelList(), vertex.GetDefaultModelPricing()},
	} {
		t.Run(provider.name, func(t *testing.T) {
			for _, name := range []string{"gemini-4-carbon", "gemini-4-hydrogen"} {
				require.Contains(t, provider.models, name)
				cfg, ok := provider.pricing[name]
				require.True(t, ok, "published models must have explicit pricing")
				want := geminiOpenaiCompatible.ModelRatios[name]
				if provider.name != "vertex" {
					require.Equal(t, want.Ratio, cfg.Ratio)
					require.Equal(t, want.CompletionRatio, cfg.CompletionRatio)
					require.Equal(t, want.CachedInputRatio, cfg.CachedInputRatio)
				} else {
					// Vertex may override shared tariffs: its introductory credits
					// and gross invoice rates differ from Developer API discounts.
					require.Positive(t, cfg.Ratio)
					require.Positive(t, cfg.CompletionRatio)
					require.Positive(t, cfg.CachedInputRatio)
					if name == "gemini-4-carbon" {
						require.InDelta(t, 4.0, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
						require.InDelta(t, 20.0, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
						require.InDelta(t, 0.20, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
						require.Empty(t, cfg.TimeWindows, "Cloud account credits must not become a Developer API tariff window")
					}
				}
				require.Equal(t, want.SupportedReasoningEfforts, cfg.SupportedReasoningEfforts)
			}
		})
	}
	// Vertex's provider override must not mutate the shared Developer tariff.
	require.InDelta(t, 2.0, geminiOpenaiCompatible.ModelRatios["gemini-4-carbon"].Ratio/ratio.MilliTokensUsd, 1e-12)
	require.Len(t, geminiOpenaiCompatible.ModelRatios["gemini-4-carbon"].TimeWindows, 1)
	for _, name := range []string{"gemini-4-carbon", "gemini-4-hydrogen"} {
		require.NotNil(t, vertexai.GetAdaptor(name), "shared discovery must also have a dispatch entry")
		for _, stream := range []bool{false, true} {
			requestMeta := &meta.Meta{ActualModelName: name, Mode: relaymode.ChatCompletions, IsStream: stream}
			requestMeta.Config.VertexAIProjectID = "catalog-fixture"
			url, err := vertex.GetRequestURL(requestMeta)
			require.NoError(t, err)
			action := "generateContent"
			if stream {
				action = "streamGenerateContent?alt=sse"
			}
			require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/catalog-fixture/locations/global/publishers/google/models/"+name+":"+action, url)
		}
	}
}
