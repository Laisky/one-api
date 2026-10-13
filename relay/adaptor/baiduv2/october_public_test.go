package baiduv2_test

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/adaptor/baiduv2"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

// TestOctoberQianfanPublishedChat verifies prices, exact IDs and the actual
// compatible transport using t. It returns nothing and never calls upstream.
func TestOctoberQianfanPublishedChat(t *testing.T) {
	t.Parallel()
	for id, prices := range map[string][3]float64{"glm-5.3-flash": {.8, .23, 2.8}, "glm-5.3": {8, 2, 28}, "glm-5.2": {8, 2, 28}} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			native := &baiduv2.Adaptor{}
			cfg, ok := native.GetDefaultModelPricing()[id]
			require.True(t, ok)
			require.Contains(t, native.GetModelList(), id)
			require.Contains(t, baiduv2.ModelList, id)
			require.EqualValues(t, 1000000, cfg.ContextLength)
			require.EqualValues(t, 131072, cfg.MaxOutputTokens)
			require.Empty(t, cfg.SupportedReasoningEfforts)
			require.Zero(t, cfg.MaxTokens)
			require.Empty(t, cfg.TimeWindows)
			require.Empty(t, cfg.Tiers)
			require.Equal(t, []string{"text"}, cfg.InputModalities)
			require.Equal(t, []string{"reasoning"}, cfg.SupportedFeatures)
			m := &meta.Meta{ChannelType: channeltype.BaiduV2, Mode: relaymode.ChatCompletions, BaseURL: "https://qianfan.baidubce.com", OriginModelName: id, ActualModelName: id}
			a := &openai.Adaptor{}
			a.Init(m)
			require.Contains(t, a.GetModelList(), id)
			require.InDelta(t, prices[0], a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, prices[1], a.GetDefaultModelPricing()[id].CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, prices[2], a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12)
			url, err := a.GetRequestURL(m)
			require.NoError(t, err)
			require.Equal(t, "https://qianfan.baidubce.com/v2/chat/completions", url)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			meta.Set2Context(c, m)
			req := &model.GeneralOpenAIRequest{Model: id, MaxTokens: 4096, Messages: []model.Message{{Role: "user", Content: "Hello"}}}
			converted, err := a.ConvertRequest(c, relaymode.ChatCompletions, req)
			require.NoError(t, err)
			data, err := json.Marshal(converted)
			require.NoError(t, err)
			var got model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal(data, &got))
			require.Equal(t, id, got.Model)
			require.Zero(t, got.MaxTokens)
			require.NotNil(t, got.MaxCompletionTokens)
			require.Equal(t, 4096, *got.MaxCompletionTokens)
			require.Equal(t, req.Messages, got.Messages)
			copy := cfg.Clone()
			copy.InputModalities[0] = "changed"
			require.Equal(t, "text", native.GetDefaultModelPricing()[id].InputModalities[0])
		})
	}
	historical := (&baiduv2.Adaptor{}).GetDefaultModelPricing()["GLM-5.3"]
	require.InDelta(t, 8, historical.Ratio/ratio.MilliTokensRmb, 1e-12)
	require.Len(t, historical.TimeWindows, 3)
	require.NotContains(t, baiduv2.ModelList, "glm-5.2-fast", "pricing display names alone do not establish an API ID")
}

// TestOctoberQianfanQwenTiers checks exact independent host tariffs on both
// sides of the documented prompt threshold using t. It returns nothing.
func TestOctoberQianfanQwenTiers(t *testing.T) {
	t.Parallel()
	for id, rates := range map[string][4]float64{
		"qwen3.5-397b-a17b": {1.2, 7.2, 3, 18}, "qwen3.5-122b-a10b": {.8, 6.4, 2, 16},
		"qwen3.5-27b": {.6, 4.8, 1.8, 14.4}, "qwen3.5-35b-a3b": {.4, 3.2, 1.6, 12.8},
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := (&baiduv2.Adaptor{}).GetDefaultModelPricing()[id]
			require.True(t, ok)
			require.Contains(t, baiduv2.ModelList, id)
			require.EqualValues(t, 256000, cfg.ContextLength)
			require.EqualValues(t, 65536, cfg.MaxOutputTokens)
			require.Zero(t, cfg.CachedInputRatio)
			require.Zero(t, cfg.MaxTokens)
			require.Len(t, cfg.Tiers, 1)
			require.Empty(t, cfg.TimeWindows)
			for _, tokens := range []int{127999, 128000, 128001, 256000} {
				resolved := pricing.ResolveEffectivePricingFromConfig(tokens, cfg)
				offset := 0
				if tokens > 128000 {
					offset = 2
				}
				require.InDelta(t, rates[offset], resolved.InputRatio/ratio.MilliTokensRmb, 1e-12)
				require.InDelta(t, rates[offset+1], resolved.OutputRatio/ratio.MilliTokensRmb, 1e-12)
			}
			cloned := cfg.Clone()
			cloned.Tiers[0].Ratio = 999
			require.NotEqual(t, cloned.Tiers[0].Ratio, (&baiduv2.Adaptor{}).GetDefaultModelPricing()[id].Tiers[0].Ratio)
		})
	}
}
