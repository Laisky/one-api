package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

func TestDeepSeekWSConservativeAcceptedCreatesAndObservedCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	channel := &model.Channel{Type: channeltype.DeepSeek}
	require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{
		Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: 17}},
	}))
	c.Set(ctxkey.ChannelModel, channel)
	meta := &metalib.Meta{ChannelType: channeltype.DeepSeek, BaseURL: "https://api.deepseek.com", ActualModelName: "deepseek-v4-pro"}
	meter := newDeepSeekWSConservativeMeter(c)
	frame := []byte(`{"type":"response.create","tools":[{"type":"web_search"}]}`)
	meter.onForwardedCreate(frame, "wss://api.deepseek.com/v1/responses")
	meter.onForwardedCreate(frame, "wss://api.deepseek.com/v1/responses")
	meter.onUpstreamEvent([]byte(`{"type":"response.output_item.added","item":{"id":"ws1","type":"web_search_call"}}`))
	meter.onUpstreamEvent([]byte(`{"type":"response.output_item.done","item":{"id":"ws1","type":"web_search_call"}}`))
	meter.onUpstreamEvent([]byte(`{"type":"response.completed","response":{"output":[{"id":"ws1","type":"web_search_call"}]}}`))
	var usage *relaymodel.Usage
	meter.settle(c, meta, &usage)
	require.NotNil(t, usage)
	require.EqualValues(t, 340, usage.ToolsCost)
	require.Equal(t, "deepseek_conservative_tool_attempt_floor", usage.BillingEstimateReason)
	raw, ok := c.Get(ctxkey.ToolInvocationSummary)
	require.True(t, ok)
	summary := raw.(*model.ToolUsageSummary)
	require.Equal(t, 20, summary.Counts["web_search"])
	require.EqualValues(t, 340, summary.TotalCost)

	other := newDeepSeekWSConservativeMeter(c)
	other.onForwardedCreate(frame, "wss://api.openai.com/v1/responses")
	var uncharged *relaymodel.Usage
	other.settle(c, meta, &uncharged)
	require.Nil(t, uncharged)
}
