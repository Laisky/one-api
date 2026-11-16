package alibailian_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/ali"
	"github.com/Laisky/one-api/relay/adaptor/alibailian"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestNovemberAlibabaProviderDefaults checks published Beijing prices through
// both public adaptor surfaces using t. It returns nothing and calls no upstream.
func TestNovemberAlibabaProviderDefaults(t *testing.T) {
	t.Parallel()
	for _, provider := range []struct {
		name string
		a    adaptor.Adaptor
	}{
		{"native", &ali.Adaptor{}},
		{"compatible", &openai.Adaptor{ChannelType: channeltype.AliBailian}},
	} {
		t.Run(provider.name, func(t *testing.T) {
			t.Parallel()
			for id, rates := range map[string][3]float64{
				"qwen4-max":                       {10, .5, 30},
				"MiniMax/MiniMax-M3.1-Flash":      {1, .1, 4},
				"kimi-k3.1-pro":                   {13, 1.3, 68},
				"kimi/kimi-k3.1-pro":              {13, 1.3, 68},
				"kimi/kimi-k3.1-pro-highspeed":    {26, 1.3, 136},
				"glm-5.3-prime":                   {16, 4, 56},
				"xiaomi/mimo-v2.6-pro-ultraspeed": {30, 0, 60},
				"deepseek-v4.1-flash-1017":        {1, .02, 4},
			} {
				cfg, ok := provider.a.GetDefaultModelPricing()[id]
				require.True(t, ok, id)
				require.Contains(t, provider.a.GetModelList(), id)
				require.InDelta(t, rates[0], provider.a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12, id)
				require.InDelta(t, rates[1], cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12, id)
				require.InDelta(t, rates[2], provider.a.GetModelRatio(id)*provider.a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12, id)
				require.Empty(t, cfg.SupportedReasoningEfforts, "unsupported gateway effort forwarding must not be advertised")
				require.Zero(t, cfg.CacheWrite5mRatio, "TTL-specific cache write receipts are not yet qualified")
				require.Zero(t, cfg.CacheWrite1hRatio)
				if provider.name == "native" {
					require.Equal(t, []string{"text"}, cfg.InputModalities)
				}
			}
			for _, tokens := range []int{512000, 512001, 1_000_000} {
				cfg := provider.a.GetDefaultModelPricing()["MiniMax/MiniMax-M3"]
				resolved := pricing.ResolveEffectivePricingFromConfig(tokens, cfg)
				input := 2.1
				if tokens > 512000 {
					input = 4.2
				}
				require.InDelta(t, input, resolved.InputRatio/ratio.MilliTokensRmb, 1e-12)
				require.InDelta(t, input*.2, resolved.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
				require.InDelta(t, input*4, resolved.OutputRatio/ratio.MilliTokensRmb, 1e-12)
			}
			for _, hour := range []int{0, 14} {
				cfg := provider.a.GetDefaultModelPricing()["deepseek-v4.1-flash-1017"]
				resolved := pricing.ResolveEffectivePricingFromConfig(512, pricing.ApplyTimeWindow(cfg, time.Date(2026, 11, 15, hour, 0, 0, 0, time.UTC)))
				input := 2.0
				if hour == 14 {
					input = 1
				}
				require.InDelta(t, input, resolved.InputRatio/ratio.MilliTokensRmb, 1e-12)
				require.InDelta(t, input*.02, resolved.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
				require.InDelta(t, input*4, resolved.OutputRatio/ratio.MilliTokensRmb, 1e-12)
			}
			require.NotContains(t, provider.a.GetModelList(), "qwen4-max-preview", "Token Plan preview does not publish a pay-as-you-go tariff")
		})
	}
}

// TestNovemberAlibabaRequestCompatibility checks the native text route and
// compatible multimodal preservation for the new IDs using t. It returns nothing.
func TestNovemberAlibabaRequestCompatibility(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"qwen4-max", "MiniMax/MiniMax-M3.1-Flash", "kimi-k3.1-pro", "kimi/kimi-k3.1-pro", "kimi/kimi-k3.1-pro-highspeed", "glm-5.3-prime", "xiaomi/mimo-v2.6-pro-ultraspeed", "deepseek-v4.1-flash-1017"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			native := &ali.Adaptor{}
			req := &model.GeneralOpenAIRequest{Model: id, MaxTokens: 2048, Messages: []model.Message{{Role: "user", Content: "Hello"}}}
			converted, err := native.ConvertRequest(nil, relaymode.ChatCompletions, req)
			require.NoError(t, err)
			chat, ok := converted.(*ali.ChatRequest)
			require.True(t, ok)
			require.Equal(t, id, chat.Model)
			require.Equal(t, "Hello", chat.Input.Messages[0].Content)
			require.Equal(t, 2048, chat.Parameters.MaxTokens)
			m := &meta.Meta{ChannelType: channeltype.AliBailian, ActualModelName: id, OriginModelName: id, Mode: relaymode.ChatCompletions, BaseURL: "https://dashscope.aliyuncs.com"}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			meta.Set2Context(c, m)
			a := &openai.Adaptor{ChannelType: channeltype.AliBailian}
			if id == "qwen4-max" || id == "MiniMax/MiniMax-M3.1-Flash" {
				req.Messages[0].Content = []any{
					map[string]any{"type": "text", "text": "Describe these."},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/image.png"}},
					map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/video.mp4"}},
				}
			}
			converted, err = a.ConvertRequest(c, relaymode.ChatCompletions, req)
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var got model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal(encoded, &got))
			require.Equal(t, id, got.Model)
			require.Equal(t, req.Messages, got.Messages)
			require.NotNil(t, got.MaxCompletionTokens)
			require.Equal(t, 2048, *got.MaxCompletionTokens)
			url, err := a.GetRequestURL(m)
			require.NoError(t, err)
			require.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", url)
			require.Contains(t, alibailian.ModelList, id)
		})
	}
	require.Equal(t, []string{"text", "image"}, alibailian.ModelRatios["kimi-k3.1-pro"].InputModalities)
	require.NotSame(t, &ali.ModelRatios["qwen4-max"].InputModalities[0], &alibailian.ModelRatios["qwen4-max"].InputModalities[0])
}
