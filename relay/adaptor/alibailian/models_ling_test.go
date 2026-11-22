package alibailian_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/ali"
	"github.com/Laisky/one-api/relay/adaptor/alibailian"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestLingPublishedCompatibleCatalog verifies public discovery and Beijing
// promotion/cache billing through the compatible adaptor using t, returning nothing.
func TestLingPublishedCompatibleCatalog(t *testing.T) {
	t.Parallel()
	const id = "antdigital/Ling-3.1-flash"
	a := &openai.Adaptor{ChannelType: channeltype.AliBailian}
	cfg, exists := a.GetDefaultModelPricing()[id]
	require.True(t, exists)
	require.Contains(t, a.GetModelList(), id)
	require.Contains(t, alibailian.ModelList, id)
	require.InDelta(t, .5, a.GetModelRatio(id)/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, .1, cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 1.5, a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensRmb, 1e-12)
	require.EqualValues(t, 1_048_576, cfg.ContextLength)
	require.EqualValues(t, 262_144, cfg.MaxOutputTokens)
	require.Equal(t, []string{"text"}, cfg.InputModalities)
	require.Contains(t, cfg.SupportedFeatures, "json_mode")
	require.NotContains(t, cfg.SupportedFeatures, "structured_outputs")
	require.NotContains(t, cfg.SupportedFeatures, "web_search")
	require.Empty(t, cfg.SupportedReasoningEfforts)
	require.Empty(t, cfg.TimeWindows, "a promotion expiry must not be invented")
	require.NotContains(t, (&ali.Adaptor{}).GetModelList(), id, "upstream documents only the compatible transport")
}

// TestLingCompatibleRequestPreservesPublishedID verifies that the existing
// request path preserves Ling's ID and JSON mode using t, without contacting upstream.
func TestLingCompatibleRequestPreservesPublishedID(t *testing.T) {
	t.Parallel()
	const id = "antdigital/Ling-3.1-flash"
	m := &meta.Meta{
		ChannelType: channeltype.AliBailian, ActualModelName: id, OriginModelName: id,
		Mode: relaymode.ChatCompletions, BaseURL: "https://example-workspace.cn-beijing.maas.aliyuncs.com",
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	meta.Set2Context(c, m)
	a := &openai.Adaptor{ChannelType: channeltype.AliBailian}
	req := &model.GeneralOpenAIRequest{
		Model: id, Messages: []model.Message{{Role: "user", Content: "Return a JSON object."}},
		ResponseFormat: &model.ResponseFormat{Type: "json_object"},
	}
	converted, err := a.ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	var got model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal(encoded, &got))
	require.Equal(t, id, got.Model)
	require.Equal(t, req.Messages, got.Messages)
	require.Equal(t, req.ResponseFormat, got.ResponseFormat)
	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	require.Equal(t, m.BaseURL+"/compatible-mode/v1/chat/completions", url)
}
