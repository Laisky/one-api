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

// TestGPT6SolLunaCatalog verifies the public adapter surfaces expose published
// model IDs, Standard prices, token limits, and capabilities without credentials.
func TestGPT6SolLunaCatalog(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                  string
		input, cached, output float64
		cacheWrite            float64
	}{
		{"gpt-6-sol", 2, 0.2, 10, 2.5},
		{"gpt-6-luna", 0.1, 0.01, 0.5, 0.125},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Adaptor{ChannelType: channeltype.OpenAI}
			require.Contains(t, a.GetModelList(), tc.name)
			require.Contains(t, ModelList, tc.name)
			cfg, ok := a.GetDefaultModelPricing()[tc.name]
			require.True(t, ok)
			require.InDelta(t, tc.input, a.GetModelRatio(tc.name)/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.name)*a.GetCompletionRatio(tc.name)/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, tc.cacheWrite, cfg.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9)
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
			require.Nil(t, cfg.Audio)
			require.Nil(t, cfg.Video)
		})
	}
}

// TestGPT6SolLunaReasoningNormalization verifies all published effort levels
// survive normalization, including mapped model names and legacy client input.
func TestGPT6SolLunaReasoningNormalization(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"gpt-6-sol", "gpt-6-luna"} {
		for _, modelName := range []string{name, " " + strings.ToUpper(name) + " "} {
			require.True(t, isModelSupportedReasoning(modelName))
			require.False(t, isMediumOnlyReasoningModel(modelName))
			require.Equal(t, "medium", *normalizeReasoningEffortForModel(modelName, nil))
			for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
				require.True(t, isReasoningEffortAllowedForModel(modelName, effort))
				requested := effort
				require.Equal(t, effort, *normalizeReasoningEffortForModel(modelName, &requested))
			}
			for _, tc := range []struct{ requested, expected string }{
				{"minimal", "none"}, {" MAX ", "max"}, {"ultra", "medium"}, {"", "medium"},
			} {
				requested := tc.requested
				require.Equal(t, tc.expected, *normalizeReasoningEffortForModel(modelName, &requested))
			}
			require.False(t, isReasoningEffortAllowedForModel(modelName, "ultra"))
		}
	}
	// Do not accidentally add a non-reasoning mode to the existing Astra model.
	require.False(t, isReasoningEffortAllowedForModel("gpt-6-astra", "none"))
	require.False(t, isReasoningEffortAllowedForModel("gpt-6-astra", "minimal"))
}

// TestGPT6SolLunaRequestTransformations verifies model metadata drives the real
// transformation and Responses routing paths without losing tools or effort.
func TestGPT6SolLunaRequestTransformations(t *testing.T) {
	for _, name := range []string{"gpt-6-sol", "gpt-6-luna"} {
		for _, effort := range []string{"none", "xhigh", "max"} {
			t.Run(name+"/"+effort, func(t *testing.T) {
				a := &Adaptor{ChannelType: channeltype.OpenAI}
				info := &relaymeta.Meta{
					ChannelType: channeltype.OpenAI, ActualModelName: name,
					Mode: relaymode.ChatCompletions, BaseURL: "https://api.openai.com",
					RequestURLPath: "/v1/chat/completions",
				}
				requested := effort
				req := &model.GeneralOpenAIRequest{
					Model: "tenant-model-alias", ReasoningEffort: &requested, MaxTokens: 1024,
					Messages: []model.Message{{Role: "user", Content: "Search for the release notes."}},
					Tools:    []model.Tool{{Type: "web_search"}},
				}
				require.True(t, shouldForceResponseAPI(info))
				url, err := a.GetRequestURL(info)
				require.NoError(t, err)
				require.Equal(t, "https://api.openai.com/v1/responses", url)
				require.NoError(t, a.applyRequestTransformations(info, req))
				require.NotNil(t, req.ReasoningEffort)
				require.Equal(t, effort, *req.ReasoningEffort, "use the actual upstream model's effort metadata")
				require.NotNil(t, req.MaxCompletionTokens)
				require.Equal(t, 1024, *req.MaxCompletionTokens)
				require.Zero(t, req.MaxTokens)
				require.Equal(t, []model.Tool{{Type: "web_search"}}, req.Tools)
				require.Equal(t, []model.Message{{Role: "user", Content: "Search for the release notes."}}, req.Messages)
			})
		}
	}
}
