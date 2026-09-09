package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPTImage25ReviewRelayAccounting checks admission, actual pre-debit, HTTP
// decoding, final balances, and persisted costs together. Parameters: t runs the
// cases. Returns: none. It is serial because the existing fixture swaps global DBs.
func TestGPTImage25ReviewRelayAccounting(t *testing.T) {
	const cachedUsage = `{"input_tokens":2000,"output_tokens":1000,"total_tokens":3000,"input_tokens_details":{"text_tokens":1000,"image_tokens":1000,"cached_tokens":1000,"cached_tokens_details":{"text_tokens":0,"image_tokens":1000}}}`
	for _, tc := range []struct {
		name, model, usage string
		tariff             float64
		balance            int64
		count, status      int
		reserved, charged  int64
		fallback           bool
	}{
		{"unconfigured_sunburst", "gpt-image-2.5-sunburst", `{}`, 0, 1_000_000, 1, 503, 0, 0, false},
		{"unconfigured_sunburst_snapshot", "gpt-image-2.5-sunburst-2026-09-08", `{}`, 0, 1_000_000, 1, 503, 0, 0, false},
		{"unconfigured_flare", "gpt-image-2.5-flare", `{}`, 0, 1_000_000, 1, 503, 0, 0, false},
		{"unconfigured_flare_snapshot", "gpt-image-2.5-flare-2026-09-08", `{}`, 0, 1_000_000, 1, 503, 0, 0, false},
		{"insufficient_credit", "gpt-image-2.5-sunburst", cachedUsage, 0.1, 3, 1, 403, 0, 0, false},
		{"reserve_then_refund_unused", "gpt-image-2.5-sunburst", cachedUsage, 0.1, 1_000_000, 2, 200, 100_000, 18_500, false},
		{"charge_above_reserve", "gpt-image-2.5-flare", `{"input_tokens":1000,"output_tokens":10000}`, 0.1, 1_000_000, 1, 200, 50_000, 152_500, false},
		{"missing_usage", "gpt-image-2.5-flare", `null`, 0.1, 1_000_000, 1, 200, 50_000, 50_000, true},
		{"empty_usage", "gpt-image-2.5-sunburst", `{}`, 0.1, 1_000_000, 1, 200, 50_000, 50_000, true},
		{"ambiguous_cache_split", "gpt-image-2.5-flare", `{"input_tokens":2000,"output_tokens":1000,"input_tokens_details":{"text_tokens":1000,"image_tokens":1000,"cached_tokens":1000}}`, 0.1, 1_000_000, 1, 200, 50_000, 50_000, true},
		{"channel_pricing_override", "gpt-image-2.5-sunburst", cachedUsage, 0.1, 1_000_000, 1, 200, 50_000, 26_000, false},
		{"legacy_per_image_control", "dall-e-3", `{}`, 0, 1_000_000, 1, 200, 20_000, 20_000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := setupCacheBillingLogTest(t)
			defer cleanup()
			require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.UserRequestCost{}))
			require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("quota", tc.balance).Error)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("remain_quota", tc.balance).Error)
			oldLog := config.IsLogConsumeEnabled()
			config.SetLogConsumeEnabled(true)
			defer config.SetLogConsumeEnabled(oldLog)
			oldBatch := config.BatchUpdateEnabled
			config.BatchUpdateEnabled = false
			defer func() { config.BatchUpdateEnabled = oldBatch }()

			var calls atomic.Int32
			var atUpstreamUser, atUpstreamToken atomic.Int64
			var upstreamDBError atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var user model.User
				var token model.Token
				if model.DB.First(&user, 1).Error != nil || model.DB.First(&token, 1).Error != nil {
					upstreamDBError.Store(true)
				}
				atUpstreamUser.Store(user.Quota)
				atUpstreamToken.Store(token.RemainQuota)
				w.Header().Set("Content-Type", "application/json")
				_, err := fmt.Fprintf(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":%s}`, tc.usage)
				if err != nil {
					upstreamDBError.Store(true)
				}
			}))
			defer upstream.Close()
			channel := &model.Channel{Id: 1, Type: channeltype.OpenAI, Name: "image25-review", Status: model.ChannelStatusEnabled}
			if tc.tariff > 0 {
				pricingJSON := fmt.Sprintf(`{%q:{"image":{"price_per_image_usd":%g}}}`, tc.model, tc.tariff)
				if tc.name == "channel_pricing_override" {
					pricingJSON = fmt.Sprintf(`{%q:{"ratio":5,"completion_ratio":4,"cached_input_ratio":0.5,"image":{"price_per_image_usd":0.1,"prompt_ratio":2}}}`, tc.model)
				}
				channel.ModelConfigs = &pricingJSON
			}
			require.NoError(t, model.DB.Create(channel).Error)
			if tc.tariff > 0 {
				pricing := channel.GetModelPriceConfigsWithContext(context.Background())
				require.NotNil(t, pricing[tc.model].Image, "the real channel parser must recognize the test override")
				require.Equal(t, tc.tariff, pricing[tc.model].Image.PricePerImageUsd)
			}
			body := fmt.Sprintf(`{"model":%q,"prompt":"A lighthouse","n":%d}`, tc.model, tc.count)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			gmw.SetLogger(c, logger.Logger)
			requestID := "image25-review-" + tc.name
			c.Set(ctxkey.Id, 1)
			c.Set(ctxkey.TokenId, 1)
			c.Set(ctxkey.TokenName, "relay-cache-log-token")
			c.Set(ctxkey.ChannelId, 1)
			c.Set(ctxkey.ChannelRatio, 1.0)
			c.Set(ctxkey.ChannelModel, channel)
			c.Set(ctxkey.ContentType, "application/json")
			c.Set(ctxkey.RequestId, requestID)
			mode := relaymode.GetByPath("/v1/images/generations")
			metalib.Set2Context(c, &metalib.Meta{
				Mode: mode, ChannelType: channeltype.OpenAI, APIType: channeltype.ToAPIType(channeltype.OpenAI),
				ChannelId: 1, UserId: 1, TokenId: 1, TokenName: "relay-cache-log-token",
				BaseURL: upstream.URL, APIKey: "test-only", OriginModelName: tc.model,
				ActualModelName: tc.model, RequestURLPath: "/v1/images/generations", StartTime: time.Now(),
			})
			gotErr := RelayImageHelper(c, mode)
			if tc.status != http.StatusOK {
				require.NotNil(t, gotErr)
				require.Equal(t, tc.status, gotErr.StatusCode)
				if tc.status == http.StatusServiceUnavailable {
					require.Equal(t, "image_billing_not_configured", gotErr.Code)
				}
				require.Zero(t, calls.Load(), "rejected requests must never reach a paid upstream")
			} else {
				require.Nil(t, gotErr)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Contains(t, recorder.Body.String(), "AQ==")
				require.Equal(t, int32(1), calls.Load())
				require.False(t, upstreamDBError.Load())
				require.Equal(t, tc.balance-tc.reserved, atUpstreamUser.Load(), "reserve user quota before HTTP")
				require.Equal(t, tc.balance-tc.reserved, atUpstreamToken.Load(), "reserve token quota before HTTP")
				var cost model.UserRequestCost
				require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&cost).Error)
				require.Equal(t, tc.charged, cost.Quota)
				var entry model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).First(&entry).Error)
				require.Equal(t, int(tc.charged), entry.Quota)
				if tc.fallback {
					require.Contains(t, entry.Content, "billing_source=configured_fallback")
				}
			}
			var user model.User
			var token model.Token
			require.NoError(t, model.DB.First(&user, 1).Error)
			require.NoError(t, model.DB.First(&token, 1).Error)
			require.Equal(t, tc.balance-tc.charged, user.Quota)
			require.Equal(t, tc.balance-tc.charged, token.RemainQuota)
		})
	}
}
