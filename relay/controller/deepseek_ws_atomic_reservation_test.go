package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestDeepSeekWSConservativeAtomicConcurrentFunding proves that two independent
// sessions cannot both admit a paid create against the same finite token and
// owner balances. No real provider is contacted.
func TestDeepSeekWSConservativeAtomicConcurrentFunding(t *testing.T) {
	_ = wsSettlementSetup(t)
	const ownerBudget int64 = 200
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fallbackUserID).Update("quota", ownerBudget).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).
		Updates(map[string]any{"remain_quota": ownerBudget, "used_quota": 0, "unlimited_quota": false}).Error)
	require.NoError(t, model.CacheUpdateUserQuota(context.Background(), fallbackUserID))

	channel := &model.Channel{Type: channeltype.OpenAI}
	require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{
		Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: 17}},
	}))
	meta := &metalib.Meta{ChannelType: channeltype.OpenAI,
		BaseURL: "https://api.deepseek.com", ActualModelName: "deepseek-v4-pro",
		Mode: relaymode.ResponseAPI, UserId: fallbackUserID, TokenId: fallbackTokenID}
	const wire = `{"type":"response.create","tools":[{"type":"web_search"}],"input":"hello"}`
	results := make([]error, 2)
	funded := make([]atomic.Int64, 2)
	admit := make([]func(context.Context, []byte, string) error, 2)
	for i := range admit {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses?model=deepseek-v4-pro", nil)
		c.Set(ctxkey.ChannelModel, channel)
		admit[i] = responseWebSocketToolAdmission(c, meta, &funded[i])
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range admit {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = admit[i](context.Background(), []byte(wire), "wss://api.deepseek.com/v1/responses")
		}(i)
	}
	close(start)
	wg.Wait()

	var admitted int
	var debit int64
	for i := range results {
		if results[i] == nil {
			admitted++
		}
		debit += funded[i].Load()
	}
	require.Equal(t, 1, admitted, "the second concurrent socket must not spend the same quota")
	require.EqualValues(t, 170, debit)
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.EqualValues(t, 170, token.UsedQuota)
	require.EqualValues(t, ownerBudget-170, token.RemainQuota)
	require.EqualValues(t, ownerBudget-170, reloadUserQuota(t))
}

// TestDeepSeekWSConservativeMalformedCreateHasNoHold ensures validation rejects
// a duplicate declaration before touching durable quota.
func TestDeepSeekWSConservativeMalformedCreateHasNoHold(t *testing.T) {
	_ = wsSettlementSetup(t)
	channel := &model.Channel{Type: channeltype.OpenAI}
	require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{
		Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: 17}},
	}))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Set(ctxkey.ChannelModel, channel)
	meta := &metalib.Meta{ChannelType: channeltype.OpenAI,
		BaseURL: "https://api.deepseek.com", ActualModelName: "deepseek-v4-pro",
		Mode: relaymode.ResponseAPI, UserId: fallbackUserID, TokenId: fallbackTokenID}
	var funded atomic.Int64
	admission := responseWebSocketToolAdmission(c, meta, &funded)
	err := admission(context.Background(), []byte(`{"type":"response.create","tools":[{"type":"web_search"}],"tools":[]}`), "wss://api.deepseek.com/v1/responses")
	require.Error(t, err)
	require.Zero(t, funded.Load())
}
