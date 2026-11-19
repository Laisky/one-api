package baiduv2_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/baiduv2"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestNovemberQianfanPromotionBoundaries checks both daily transitions and the
// advertised expiry using UTC request instants. It accepts t and returns nothing.
func TestNovemberQianfanPromotionBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		at, window            string
		input, cached, output float64
	}{
		{"2026-11-29T23:59:59Z", "ernie55-preview-promotion-offpeak", .5, .05, 2},
		{"2026-11-30T00:00:00Z", "ernie55-preview-promotion-peak", 1, .1, 4},
		{"2026-11-30T13:59:59Z", "ernie55-preview-promotion-peak", 1, .1, 4},
		{"2026-11-30T14:00:00Z", "ernie55-preview-promotion-offpeak", .5, .05, 2},
		{"2026-11-30T15:59:59.999999999Z", "ernie55-preview-promotion-offpeak", .5, .05, 2},
		{"2026-11-30T16:00:00Z", "", 1, .1, 4},
		{"2026-12-01T00:00:00Z", "qianfan-peak", 2, .2, 8},
		{"2026-12-01T14:00:00Z", "", 1, .1, 4},
	} {
		t.Run(tc.at, func(t *testing.T) {
			t.Parallel()
			at, err := time.Parse(time.RFC3339Nano, tc.at)
			require.NoError(t, err)
			cfg := baiduv2.ModelRatios["ernie-5.5-flash-preview"]
			require.Equal(t, tc.window, pricing.ActiveTimeWindowName(cfg, at))
			resolved := pricing.ResolveEffectivePricingFromConfig(1024, pricing.ApplyTimeWindow(cfg, at))
			require.InDelta(t, tc.input, resolved.InputRatio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, tc.cached, resolved.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, tc.output, resolved.OutputRatio/ratio.MilliTokensRmb, 1e-12)
		})
	}
}

// TestNovemberQianfanPublishedIDs checks exact public IDs and compatible request
// conversion without claiming unforwarded effort controls. It takes t and returns nothing.
func TestNovemberQianfanPublishedIDs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id                 string
		context, maxOutput int32
	}{
		{"ernie-5.5-flash-preview", 1_000_000, 1_000_000},
		{"ernie-light-dt-1.0", 1_000_000, 128_000},
		{"ernie-5.1", 128_000, 65_536},
		{"deepseek-v4.1-flash", 1_000_000, 393_216},
		{"deepseek-v4.1-flash-1017", 1_000_000, 393_216},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			a := &openai.Adaptor{ChannelType: channeltype.BaiduV2}
			cfg, ok := a.GetDefaultModelPricing()[tc.id]
			require.True(t, ok)
			require.Contains(t, a.GetModelList(), tc.id)
			require.Contains(t, baiduv2.ModelList, tc.id)
			require.Equal(t, tc.context, cfg.ContextLength)
			require.Equal(t, tc.maxOutput, cfg.MaxOutputTokens)
			require.Empty(t, cfg.SupportedReasoningEfforts)
			m := &meta.Meta{ChannelType: channeltype.BaiduV2, ActualModelName: tc.id, OriginModelName: tc.id, BaseURL: "https://qianfan.baidubce.com", Mode: relaymode.ChatCompletions}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			meta.Set2Context(c, m)
			req := &model.GeneralOpenAIRequest{Model: tc.id, MaxTokens: 4096, Messages: []model.Message{{Role: "user", Content: "Hello"}}}
			converted, err := a.ConvertRequest(c, relaymode.ChatCompletions, req)
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var got model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal(encoded, &got))
			require.Equal(t, tc.id, got.Model)
			require.Equal(t, req.Messages, got.Messages)
			require.NotNil(t, got.MaxCompletionTokens)
			require.Equal(t, 4096, *got.MaxCompletionTokens)
			url, err := a.GetRequestURL(m)
			require.NoError(t, err)
			require.Equal(t, "https://qianfan.baidubce.com/v2/chat/completions", url)
		})
	}
	require.Contains(t, baiduv2.ModelRatios, "ERNIE-5.1")
	require.Contains(t, baiduv2.ModelRatios, "DeepSeek-V4.1-Flash")
	cfg := baiduv2.ModelRatios["ernie-light-dt-1.0"]
	require.Empty(t, cfg.TimeWindows)
	require.InDelta(t, 8, cfg.Ratio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, .8, cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 28, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensRmb, 1e-12)
	for _, id := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-1017"} {
		cfg := baiduv2.ModelRatios[id]
		require.Len(t, cfg.TimeWindows, 1, "the expired holiday promotion must not be copied")
		for _, hour := range []int{0, 14} {
			resolved := pricing.ResolveEffectivePricingFromConfig(512, pricing.ApplyTimeWindow(cfg, time.Date(2026, 11, 16, hour, 0, 0, 0, time.UTC)))
			input := 2.0
			if hour == 14 {
				input = 1
			}
			require.InDelta(t, input, resolved.InputRatio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, input*.02, resolved.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, input*4, resolved.OutputRatio/ratio.MilliTokensRmb, 1e-12)
		}
	}
	for _, tokens := range []int{32000, 32001} {
		resolved := pricing.ResolveEffectivePricingFromConfig(tokens, baiduv2.ModelRatios["ernie-5.1"])
		input, output := 4.0, 18.0
		if tokens > 32000 {
			input, output = 6, 22
		}
		require.InDelta(t, input, resolved.InputRatio/ratio.MilliTokensRmb, 1e-12)
		require.InDelta(t, output, resolved.OutputRatio/ratio.MilliTokensRmb, 1e-12)
	}
}
