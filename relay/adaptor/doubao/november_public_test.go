package doubao_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/doubao"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestNovemberArkKimiPricingAndMedia checks the exact snapshot ID, independent
// Standard tariff and Base64 media preservation using t. It returns no values.
func TestNovemberArkKimiPricingAndMedia(t *testing.T) {
	t.Parallel()
	const id = "kimi-k3-1-pro-261024"
	a := &openai.Adaptor{ChannelType: channeltype.Doubao}
	cfg, ok := a.GetDefaultModelPricing()[id]
	require.True(t, ok)
	require.Contains(t, a.GetModelList(), id)
	require.Contains(t, doubao.ModelList, id)
	require.InDelta(t, 13, a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 1.3, cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 68, a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12)
	require.EqualValues(t, 1_024_000, cfg.ContextLength)
	require.EqualValues(t, 1_024_000, cfg.MaxOutputTokens)
	require.Empty(t, cfg.TimeWindows)
	require.Empty(t, cfg.Tiers)
	require.Zero(t, cfg.CacheWrite5mRatio)
	require.Zero(t, cfg.CacheWrite1hRatio)
	require.Empty(t, cfg.SupportedReasoningEfforts)
	require.NotContains(t, cfg.SupportedSamplingParameters, "temperature")
	require.Contains(t, cfg.Description, "Base64")
	const body = `{"model":"kimi-k3-1-pro-261024","messages":[{"role":"user","content":[{"type":"text","text":"Describe this media."},{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aO4sAAAAASUVORK5CYII="}},{"type":"video_url","video_url":{"url":"data:video/mp4;base64,dmlkZW8="}}]}],"max_completion_tokens":4096}`
	var req model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(body), &req))
	m := &meta.Meta{ChannelType: channeltype.Doubao, ActualModelName: id, OriginModelName: id, BaseURL: "https://ark.cn-beijing.volces.com", Mode: relaymode.ChatCompletions}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	meta.Set2Context(c, m)
	converted, err := a.ConvertRequest(c, relaymode.ChatCompletions, &req)
	require.NoError(t, err)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, body, string(encoded))
	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	require.Equal(t, "https://ark.cn-beijing.volces.com/api/v3/chat/completions", url)
	for _, deferred := range []string{"deepseek-v4-1-flash-261017", "deepseek-v4-1-flash-economy-261017"} {
		require.NotContains(t, a.GetModelList(), deferred, "completion-time tariffs are not request-start pricing windows")
	}
}
