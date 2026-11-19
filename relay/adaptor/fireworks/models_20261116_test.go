package fireworks_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/fireworks"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestNovemberFireworksCatalog verifies Standard tariffs and native ID
// preservation through Chat, Responses and Claude Messages conversion. It
// accepts t and returns no values without contacting Fireworks.
func TestNovemberFireworksCatalog(t *testing.T) {
	t.Parallel()
	a := &fireworks.Adaptor{}
	for slug, rates := range map[string][3]float64{
		"kimi-k3p1-pro":            {2, .20, 10},
		"minimax-m3p1-flash":       {.15, .015, .60},
		"deepseek-v4p1-flash-1017": {.30, .006, 1.20},
		"muse-spark-open":          {1.25, .15, 4.25},
		"ultramarine-1-flash-us":   {.15, .03, .50},
	} {
		t.Run(slug, func(t *testing.T) {
			t.Parallel()
			id := "accounts/fireworks/models/" + slug
			cfg := a.GetDefaultModelPricing()[id]
			require.Contains(t, a.GetModelList(), id)
			require.InDelta(t, rates[0], a.GetModelRatio(id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, rates[1], cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, rates[2], a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensUsd, 1e-12)
			if slug == "ultramarine-1-flash-us" {
				require.Zero(t, cfg.ContextLength, "rounded card labels do not establish an exact ceiling")
			} else {
				require.EqualValues(t, 1_048_576, cfg.ContextLength)
			}
			require.Zero(t, cfg.MaxOutputTokens)
			require.Zero(t, cfg.MaxTokens)
			require.Empty(t, cfg.SupportedReasoningEfforts, "do not infer an exact ladder from generic reasoning documentation")
			require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
			if slug == "ultramarine-1-flash-us" {
				require.NotContains(t, cfg.SupportedFeatures, "tools")
			} else {
				require.Contains(t, cfg.SupportedFeatures, "tools")
			}
			for _, mode := range []int{relaymode.ChatCompletions, relaymode.ResponseAPI} {
				var request model.GeneralOpenAIRequest
				require.NoError(t, json.Unmarshal([]byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"Describe it"},{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]}],"max_tokens":128}`), &request))
				request.Model = id
				if mode == relaymode.ResponseAPI {
					request.Messages = nil
					request.Input = "Describe one fact"
				}
				want, err := json.Marshal(request)
				require.NoError(t, err)
				converted, err := a.ConvertRequest(nil, mode, &request)
				require.NoError(t, err)
				got, err := json.Marshal(converted)
				require.NoError(t, err)
				require.JSONEq(t, string(want), string(got))
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			var claude model.ClaudeRequest
			require.NoError(t, json.Unmarshal([]byte(`{"messages":[{"role":"user","content":"Hello"}],"max_tokens":128}`), &claude))
			claude.Model = id
			converted, err := a.ConvertClaudeRequest(c, &claude)
			require.NoError(t, err)
			require.Same(t, &claude, converted)
			require.True(t, c.GetBool(ctxkey.ClaudeDirectPassthrough))
		})
	}
	require.Contains(t, a.GetModelList(), "accounts/fireworks/models/qwen3p8-2p4t-a95b")
	require.NotContains(t, a.GetModelList(), "accounts/fireworks/models/qwen3p8-max")
}
