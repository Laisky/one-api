package doubao_test

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/adaptor/doubao"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

// TestOctoberArkStandardModels checks exact IDs, rates and real compatible
// transport using t. It returns nothing and does not contact an Ark endpoint.
func TestOctoberArkStandardModels(t *testing.T) {
	t.Parallel()
	for id, rates := range map[string][3]float64{
		"doubao-seed-evolving": {6, 1.2, 30}, "doubao-seed-2-1-pro-260915": {6, 1.2, 30}, "doubao-seed-2-1-lite-260915": {.8, .16, 2.7},
		"doubao-seed-2-1-turbo-260628": {3, .6, 15}, "doubao-seed-2-1-pro-260628": {6, 1.2, 30},
		"glm-5-3-flash-260828": {.8, .23, 2.8}, "glm-5-2-260617": {8, 2, 28},
		"deepseek-v4-pro-ga-260813": {9, .30, 27}, "deepseek-v4-flash-ga-260731": {3, .10, 9}, "doubao-seed-translation-250915": {1.2, 0, 3.6},
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			native := &doubao.Adaptor{}
			cfg, ok := native.GetDefaultModelPricing()[id]
			require.True(t, ok)
			require.Contains(t, native.GetModelList(), id)
			require.Contains(t, doubao.ModelList, id)
			m := &meta.Meta{ChannelType: channeltype.Doubao, Mode: relaymode.ChatCompletions, BaseURL: "https://ark.cn-beijing.volces.com", OriginModelName: id, ActualModelName: id}
			a := &openai.Adaptor{}
			a.Init(m)
			require.Contains(t, a.GetModelList(), id)
			require.InDelta(t, rates[0], a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, rates[1], cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, rates[2], a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12)
			require.Empty(t, cfg.TimeWindows)
			require.Empty(t, cfg.Tiers)
			require.Zero(t, cfg.MaxTokens)
			require.NotContains(t, cfg.InputModalities, "audio")
			require.Nil(t, cfg.Audio)
			require.Empty(t, cfg.SupportedReasoningEfforts)
			url, err := a.GetRequestURL(m)
			require.NoError(t, err)
			require.Equal(t, "https://ark.cn-beijing.volces.com/api/v3/chat/completions", url)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			meta.Set2Context(c, m)
			req := &model.GeneralOpenAIRequest{Model: id, MaxTokens: 2048, Messages: []model.Message{{Role: "user", Content: "Hello"}}}
			converted, err := a.ConvertRequest(c, relaymode.ChatCompletions, req)
			require.NoError(t, err)
			data, err := json.Marshal(converted)
			require.NoError(t, err)
			var got model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal(data, &got))
			require.Equal(t, id, got.Model)
			require.NotNil(t, got.MaxCompletionTokens)
			require.Equal(t, 2048, *got.MaxCompletionTokens)
			require.Equal(t, req.Messages, got.Messages)
		})
	}
	cfg := (&doubao.Adaptor{}).GetDefaultModelPricing()
	require.EqualValues(t, 1024000, cfg["doubao-seed-2-1-pro-260915"].ContextLength)
	require.EqualValues(t, 256000, cfg["doubao-seed-2-1-pro-260628"].ContextLength)
	require.EqualValues(t, 256000, cfg["doubao-seed-2.1-pro"].ContextLength, "do not remap a legacy undated ID")
	require.EqualValues(t, 384000, cfg["deepseek-v4-pro-ga-260813"].MaxOutputTokens)
	require.Empty(t, cfg["doubao-seed-translation-250915"].SupportedFeatures)
	require.NotContains(t, cfg["glm-5-3-flash-260828"].SupportedFeatures, "structured_outputs")
	require.NotContains(t, cfg, "deepseek-v4-1-flash-260910", "completion-time tariffs are not request-start windows")
	require.EqualValues(t, 32001, cfg["doubao-seed-1.6"].Tiers[0].InputTokenThreshold)
	require.InDelta(t, .8, cfg["doubao-seed-1.6"].Ratio/ratio.MilliTokensRmb, 1e-12)
}
