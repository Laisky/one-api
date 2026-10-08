package anthropic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestClaudeSeptember2026PublicPricing checks current published tariffs through
// public adaptor methods, including model-specific cache discounts. It takes a
// test handle and returns nothing; it does not invoke a provider.
func TestClaudeSeptember2026PublicPricing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		model                           string
		input, output, read, five, hour float64
	}{
		{"claude-sonnet-5-5", 2, 10, 0.1, 2.5, 4},
		{"claude-sonnet-5", 2, 10, 0.2, 2.5, 4},
		{"claude-opus-5-5", 4, 20, 0.2, 5, 8},
		{"claude-opus-5", 5, 25, 0.5, 6.25, 10},
		{"claude-fable-5-1", 10, 50, 0.25, 12.5, 20},
		{"claude-mythos-5-1", 10, 50, 0.25, 12.5, 20},
		{"claude-fable-5", 10, 50, 1, 12.5, 20},
		{"claude-mythos-5", 10, 50, 1, 12.5, 20},
	} {
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			a := &Adaptor{}
			require.Contains(t, a.GetModelList(), tc.model)
			config, ok := a.GetDefaultModelPricing()[tc.model]
			require.True(t, ok)
			require.InDelta(t, tc.input*ratio.MilliTokensUsd, a.GetModelRatio(tc.model), 1e-12)
			require.InDelta(t, tc.output*ratio.MilliTokensUsd, config.Ratio*a.GetCompletionRatio(tc.model), 1e-12)
			require.InDelta(t, tc.read*ratio.MilliTokensUsd, config.CachedInputRatio, 1e-12)
			require.InDelta(t, tc.five*ratio.MilliTokensUsd, config.CacheWrite5mRatio, 1e-12)
			require.InDelta(t, tc.hour*ratio.MilliTokensUsd, config.CacheWrite1hRatio, 1e-12)
			require.Empty(t, config.TimeWindows, "do not restore expired introductory-price windows")
		})
	}
}

// TestClaudeSonnet55PublicCatalog checks the official ID, ordinary Messages
// limits, and metadata. It takes a test handle and returns nothing.
func TestClaudeSonnet55PublicCatalog(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	const modelID = "claude-sonnet-5-5"
	config, ok := a.GetDefaultModelPricing()[modelID]
	require.True(t, ok)
	require.Contains(t, a.GetModelList(), modelID)
	require.EqualValues(t, 1000000, config.ContextLength)
	require.EqualValues(t, 128000, config.MaxOutputTokens, "300K is a Batch-only beta")
	require.Zero(t, config.MaxReasoningTokens, "adaptive thinking does not take a manual budget")
	require.ElementsMatch(t, []string{"text", "image", "file"}, config.InputModalities)
	require.Equal(t, []string{"text"}, config.OutputModalities)
	require.ElementsMatch(t, []string{"stop", "max_tokens"}, config.SupportedSamplingParameters)
	require.Contains(t, config.SupportedFeatures, "tools")
	require.Contains(t, config.SupportedFeatures, "reasoning")
	for _, invented := range []string{
		"claude-sonnet-5-5-latest", "claude-sonnet-5-5-20260928",
		"claude-sonnet-5-5@20260928", "anthropic.claude-sonnet-5-5",
	} {
		require.NotContains(t, a.GetModelList(), invented)
	}
}

// TestClaudeSonnet55DefaultConvertedRequest checks the default/adaptive Chat
// wire request, including rejected sampling controls and a small output limit.
// It does not certify between_tools, forced tools, or every new upstream feature.
func TestClaudeSonnet55DefaultConvertedRequest(t *testing.T) {
	t.Parallel()
	for _, thinkingType := range []string{"omitted", "adaptive", "enabled"} {
		t.Run(thinkingType, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			temperature, topP, topK, budget := 0.5, 0.9, 40, 2048
			request := &model.GeneralOpenAIRequest{
				Model: "claude-sonnet-5-5", MaxTokens: 256,
				Temperature: &temperature, TopP: &topP, TopK: &topK,
				Messages: []model.Message{{Role: "user", Content: "Hello"}},
			}
			if thinkingType != "omitted" {
				request.Thinking = &model.Thinking{Type: thinkingType, BudgetTokens: &budget}
			}
			converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
			require.NoError(t, err)
			body, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(body, &payload))
			require.Equal(t, "claude-sonnet-5-5", payload["model"])
			require.Equal(t, float64(256), payload["max_tokens"])
			for _, field := range []string{"temperature", "top_p", "top_k"} {
				require.NotContains(t, payload, field)
			}
			if thinkingType == "omitted" {
				require.NotContains(t, payload, "thinking")
			} else {
				thinking, ok := payload["thinking"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "adaptive", thinking["type"])
				require.NotContains(t, thinking, "budget_tokens")
			}
		})
	}
}
