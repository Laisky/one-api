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

// TestClaudeOpus55PublicCatalog verifies the new model through the adapter's
// public catalog and pricing methods. It accepts a test handle and returns nothing.
func TestClaudeOpus55PublicCatalog(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	const modelID = "claude-opus-5-5"
	require.Contains(t, a.GetModelList(), modelID)
	config, ok := a.GetDefaultModelPricing()[modelID]
	require.True(t, ok)
	require.InDelta(t, 4*ratio.MilliTokensUsd, a.GetModelRatio(modelID), 1e-12)
	require.InDelta(t, 20*ratio.MilliTokensUsd, config.Ratio*a.GetCompletionRatio(modelID), 1e-12)
	require.InDelta(t, 0.2*ratio.MilliTokensUsd, config.CachedInputRatio, 1e-12)
	require.InDelta(t, 0.05, config.CachedInputRatio/config.Ratio, 1e-12)
	require.InDelta(t, 5*ratio.MilliTokensUsd, config.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 8*ratio.MilliTokensUsd, config.CacheWrite1hRatio, 1e-12)
	require.EqualValues(t, 1000000, config.ContextLength)
	require.EqualValues(t, 128000, config.MaxOutputTokens)
	require.Zero(t, config.MaxReasoningTokens)
	require.ElementsMatch(t, []string{"text", "image", "file"}, config.InputModalities)
	require.Equal(t, []string{"text"}, config.OutputModalities)
	require.Contains(t, config.SupportedFeatures, "reasoning")
	require.Contains(t, config.SupportedFeatures, "tools")
	require.ElementsMatch(t, []string{"stop", "max_tokens"}, config.SupportedSamplingParameters)
	require.Empty(t, config.TimeWindows)

	// Do not invent snapshot/latest IDs or apply the new discount to Opus 5.
	require.NotContains(t, a.GetModelList(), "claude-opus-5-5-20260922")
	require.NotContains(t, a.GetModelList(), "claude-opus-5-5-latest")
	previous := a.GetDefaultModelPricing()["claude-opus-5"]
	require.InDelta(t, 5*ratio.MilliTokensUsd, previous.Ratio, 1e-12)
	require.InDelta(t, 0.5*ratio.MilliTokensUsd, previous.CachedInputRatio, 1e-12)
}

// TestClaudeOpus55ConvertedRequest verifies the emitted request, including
// adaptive thinking and rejected sampling fields. It accepts a test handle and
// returns nothing; all requests are converted locally without network calls.
func TestClaudeOpus55ConvertedRequest(t *testing.T) {
	t.Parallel()

	for _, thinkingType := range []string{"omitted", "enabled", "disabled", "adaptive"} {
		t.Run(thinkingType, func(t *testing.T) {
			t.Parallel()

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			temperature, topP, topK, budget := 0.5, 0.9, 40, 2048
			request := &model.GeneralOpenAIRequest{
				Model:       "claude-opus-5-5",
				MaxTokens:   256,
				Temperature: &temperature,
				TopP:        &topP,
				TopK:        &topK,
				Messages:    []model.Message{{Role: "user", Content: "Hello"}},
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
			require.Equal(t, "claude-opus-5-5", payload["model"])
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
