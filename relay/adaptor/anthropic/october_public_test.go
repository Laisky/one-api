package anthropic_test

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

// TestOctoberOpusEffortMetadata verifies discoverable effort controls and exact
// native prices using t. It returns nothing and makes no upstream requests.
func TestOctoberOpusEffortMetadata(t *testing.T) {
	t.Parallel()
	a := &anthropic.Adaptor{}
	const id = "claude-opus-5-5"
	cfg, ok := a.GetDefaultModelPricing()[id]
	require.True(t, ok)
	require.Contains(t, a.GetModelList(), id)
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, cfg.SupportedReasoningEfforts)
	require.Equal(t, "medium", cfg.DefaultReasoningEffort)
	require.InDelta(t, 4, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 20, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, .2, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 5, cfg.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 8, cfg.CacheWrite1hRatio/ratio.MilliTokensUsd, 1e-12)
	require.EqualValues(t, 1000000, cfg.ContextLength)
	require.EqualValues(t, 128000, cfg.MaxOutputTokens)
	require.Zero(t, cfg.MaxReasoningTokens, "effort is not a token budget")
	require.Empty(t, cfg.TimeWindows)
	clone := cfg.Clone()
	clone.SupportedReasoningEfforts[0] = "modified"
	require.Equal(t, "low", a.GetDefaultModelPricing()[id].SupportedReasoningEfforts[0])
}

// TestOctoberOpusEffortConversion checks each declared effort survives native
// request conversion without an unsupported thinking budget. It accepts t and
// returns nothing.
func TestOctoberOpusEffortConversion(t *testing.T) {
	t.Parallel()
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			req := &model.GeneralOpenAIRequest{Model: "claude-opus-5-5", Messages: []model.Message{{Role: "user", Content: "Explain a tradeoff."}}, MaxTokens: 4096, ReasoningEffort: &effort}
			converted, err := (&anthropic.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(encoded, &payload))
			output, ok := payload["output_config"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, effort, output["effort"])
			require.NotContains(t, payload, "reasoning_effort")
			require.Equal(t, "claude-opus-5-5", payload["model"])
			require.Equal(t, float64(4096), payload["max_tokens"])
		})
	}
}
