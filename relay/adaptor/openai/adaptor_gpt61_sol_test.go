package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT61SolCatalog verifies the public OpenAI listing, prices, and capabilities
// for the published model ID without requiring an upstream account.
func TestGPT61SolCatalog(t *testing.T) {
	t.Parallel()

	const name = "gpt-6.1-sol"
	a := &Adaptor{ChannelType: channeltype.OpenAI}
	require.Contains(t, ModelList, name)
	require.Contains(t, a.GetModelList(), name)
	cfg, ok := a.GetDefaultModelPricing()[name]
	require.True(t, ok)
	require.InDelta(t, 2.0, a.GetModelRatio(name)/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 10.0, a.GetModelRatio(name)*a.GetCompletionRatio(name)/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 0.1, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 2.5, cfg.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9)
	require.Zero(t, cfg.CacheWrite1hRatio)
	require.Equal(t, int32(1_050_000), cfg.ContextLength)
	require.Equal(t, int32(128_000), cfg.MaxOutputTokens)
	require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
	require.Equal(t, []string{"text"}, cfg.OutputModalities)
	for _, feature := range []string{"reasoning", "tools", "structured_outputs", "web_search"} {
		require.Contains(t, cfg.SupportedFeatures, feature)
	}
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, cfg.SupportedReasoningEfforts)
	require.Equal(t, "medium", cfg.DefaultReasoningEffort)
	for _, parameter := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
		require.NotContains(t, cfg.SupportedSamplingParameters, parameter)
	}
	require.Nil(t, cfg.Audio)
	require.Nil(t, cfg.Image)
	require.Nil(t, cfg.Video)

	// A new model must not silently reprice the previous Sol model or remove
	// its non-reasoning mode. Third-party providers retain their own catalogs.
	require.InDelta(t, 0.2, ModelRatios["gpt-6-sol"].CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
	require.Contains(t, ModelRatios["gpt-6-sol"].SupportedReasoningEfforts, "none")
	require.Contains(t, ModelRatios["gpt-6-luna"].SupportedReasoningEfforts, "none")
	for _, provider := range []int{channeltype.Groq, channeltype.Mistral, channeltype.XAI} {
		other := &Adaptor{ChannelType: provider}
		require.NotContains(t, other.GetModelList(), name)
		require.NotContains(t, other.GetDefaultModelPricing(), name)
	}
}

// TestGPT61SolReasoningNormalization verifies supported efforts survive model-name
// normalization and invalid efforts use the existing documented default policy.
func TestGPT61SolReasoningNormalization(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"gpt-6.1-sol", " GPT-6.1-SOL "} {
		require.True(t, isModelSupportedReasoning(name))
		require.False(t, isMediumOnlyReasoningModel(name))
		require.Equal(t, "medium", *normalizeReasoningEffortForModel(name, nil))
		for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
			requested := " " + strings.ToUpper(effort) + " "
			require.True(t, isReasoningEffortAllowedForModel(name, requested))
			require.Equal(t, effort, *normalizeReasoningEffortForModel(name, &requested))
			require.False(t, modelSupportsSampling(name, &requested))
		}
		for _, effort := range []string{"none", "minimal", "ultra", ""} {
			requested := effort
			require.False(t, isReasoningEffortAllowedForModel(name, requested))
			require.Equal(t, "medium", *normalizeReasoningEffortForModel(name, &requested))
			require.False(t, modelSupportsSampling(name, &requested))
		}
	}
}

// TestGPT61SolMappedRequestTransformations verifies the actual upstream model
// controls reasoning and sampling for normalized Chat, Claude, and Responses requests.
func TestGPT61SolMappedRequestTransformations(t *testing.T) {
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
			{"low", "low"}, {"medium", "medium"}, {"high", "high"},
			{"xhigh", "xhigh"}, {"max", "max"}, {"none", "medium"}, {"minimal", "medium"},
		} {
			t.Run(surface.name+"/"+tc.requested, func(t *testing.T) {
				a := &Adaptor{ChannelType: channeltype.OpenAI}
				info := &relaymeta.Meta{
					ChannelType: channeltype.OpenAI, ActualModelName: "gpt-6.1-sol",
					Mode: surface.mode, BaseURL: "https://api.openai.com",
					RequestURLPath: surface.path,
				}
				temperature, topP, effort := 0.0, 0.9, tc.requested
				messages := []model.Message{{Role: "user", Content: "Find the release notes."}}
				tools := []model.Tool{{Type: "web_search"}}
				request := &model.GeneralOpenAIRequest{
					Model: "tenant-alias", ReasoningEffort: &effort,
					Temperature: &temperature, TopP: &topP, MaxTokens: 1024,
					Messages: messages, Tools: tools,
				}
				require.True(t, shouldForceResponseAPI(info))
				url, err := a.GetRequestURL(info)
				require.NoError(t, err)
				require.Equal(t, "https://api.openai.com/v1/responses", url)
				require.NoError(t, a.applyRequestTransformations(info, request))
				require.NotNil(t, request.ReasoningEffort)
				require.Equal(t, tc.expected, *request.ReasoningEffort)
				require.Nil(t, request.Temperature)
				require.Nil(t, request.TopP)
				require.NotNil(t, request.MaxCompletionTokens)
				require.Equal(t, 1024, *request.MaxCompletionTokens)
				require.Zero(t, request.MaxTokens)
				require.Equal(t, tools, request.Tools)
				require.Equal(t, messages, request.Messages)
			})
		}
	}
}
