package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT61TerraCatalog verifies the published OpenAI model ID and its
// Standard rates, limits, modalities and capability metadata without an API key.
func TestGPT61TerraCatalog(t *testing.T) {
	t.Parallel()

	const name = "gpt-6.1-terra"
	a := &Adaptor{ChannelType: channeltype.OpenAI}
	require.Contains(t, ModelList, name)
	require.Contains(t, a.GetModelList(), name)

	cfg, ok := a.GetDefaultModelPricing()[name]
	require.True(t, ok, "the new model must not fall back to unknown-model billing")
	require.InDelta(t, 1.0, a.GetModelRatio(name)/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 5.0, a.GetModelRatio(name)*a.GetCompletionRatio(name)/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 0.05, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 1.25, cfg.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9)
	require.Zero(t, cfg.CacheWrite1hRatio)
	require.Equal(t, int32(1_050_000), cfg.ContextLength)
	require.Equal(t, int32(128_000), cfg.MaxOutputTokens)
	require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
	require.Equal(t, []string{"text"}, cfg.OutputModalities)
	for _, feature := range []string{"reasoning", "tools", "structured_outputs", "web_search"} {
		require.Contains(t, cfg.SupportedFeatures, feature)
	}
	require.Equal(t, []string{"none", "low", "medium", "high", "xhigh", "max"}, cfg.SupportedReasoningEfforts)
	require.Equal(t, "medium", cfg.DefaultReasoningEffort)
	require.NotContains(t, cfg.SupportedSamplingParameters, "temperature")
	require.NotContains(t, cfg.SupportedSamplingParameters, "top_p")
	require.Nil(t, cfg.Audio)
	require.Nil(t, cfg.Image)
	require.Nil(t, cfg.Video)

	// The model card lists only the rolling base ID, not a dated snapshot.
	require.NotContains(t, ModelList, "gpt-6.1-terra-2026-10-26")
	require.InDelta(t, 2.0, ModelRatios["gpt-6.1-sol"].Ratio/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 0.1, ModelRatios["gpt-6.1-sol"].CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
	for _, provider := range []int{channeltype.Groq, channeltype.Mistral, channeltype.XAI} {
		other := &Adaptor{ChannelType: provider}
		require.NotContains(t, other.GetModelList(), name)
		require.NotContains(t, other.GetDefaultModelPricing(), name)
	}
}

// TestGPT61TerraReasoningNormalization verifies the exact effort ladder, the
// legacy minimal alias, default effort, and unrecognized effort handling.
func TestGPT61TerraReasoningNormalization(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"gpt-6.1-terra", " GPT-6.1-TERRA "} {
		require.True(t, isModelSupportedReasoning(name))
		require.False(t, isMediumOnlyReasoningModel(name))
		require.Equal(t, "medium", *normalizeReasoningEffortForModel(name, nil))
		for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
			requested := strings.ToUpper(effort)
			require.True(t, isReasoningEffortAllowedForModel(name, requested))
			require.Equal(t, effort, *normalizeReasoningEffortForModel(name, &requested))
		}
		for _, tc := range []struct{ requested, expected string }{
			{"minimal", "none"}, {"unexpected", "medium"}, {"", "medium"},
		} {
			requested := tc.requested
			require.Equal(t, tc.expected, *normalizeReasoningEffortForModel(name, &requested))
		}
		require.False(t, isReasoningEffortAllowedForModel(name, "unexpected"))
	}
}

// TestGPT61TerraRequestTransformations verifies the effective upstream model
// drives Chat, Claude and Responses conversion, including tool-safe routing.
func TestGPT61TerraRequestTransformations(t *testing.T) {
	for _, surface := range []struct {
		name string
		mode int
		path string
	}{
		{"chat", relaymode.ChatCompletions, "/v1/chat/completions"},
		{"claude", relaymode.ClaudeMessages, "/v1/messages"},
		{"responses", relaymode.ResponseAPI, "/v1/responses"},
	} {
		for _, tc := range []struct{ requested, expected string }{
			{"none", "none"}, {"low", "low"}, {"xhigh", "xhigh"},
			{"max", "max"}, {"minimal", "none"}, {"unsupported", "medium"},
		} {
			t.Run(surface.name+"/"+tc.requested, func(t *testing.T) {
				a := &Adaptor{ChannelType: channeltype.OpenAI}
				info := &meta.Meta{
					ChannelType: channeltype.OpenAI, ActualModelName: "gpt-6.1-terra",
					Mode: surface.mode, BaseURL: "https://api.openai.com",
					RequestURLPath: surface.path,
				}
				temperature, topP, effort := 0.3, 0.8, tc.requested
				messages := []model.Message{{Role: "user", Content: "Summarize the release notes."}}
				tools := []model.Tool{{Type: "web_search"}}
				request := &model.GeneralOpenAIRequest{
					Model: "customer-alias", ReasoningEffort: &effort,
					Temperature: &temperature, TopP: &topP, MaxTokens: 512,
					Messages: messages, Tools: tools,
				}
				require.True(t, shouldForceResponseAPI(info))
				url, err := a.GetRequestURL(info)
				require.NoError(t, err)
				require.Equal(t, "https://api.openai.com/v1/responses", url)
				require.NoError(t, a.applyRequestTransformations(info, request))
				require.NotNil(t, request.ReasoningEffort)
				require.Equal(t, tc.expected, *request.ReasoningEffort)
				if tc.expected != "none" {
					require.Nil(t, request.Temperature)
					require.Nil(t, request.TopP)
				}
				require.NotNil(t, request.MaxCompletionTokens)
				require.Equal(t, 512, *request.MaxCompletionTokens)
				require.Zero(t, request.MaxTokens)
				require.Equal(t, messages, request.Messages)
				require.Equal(t, tools, request.Tools)
			})
		}
	}
}
