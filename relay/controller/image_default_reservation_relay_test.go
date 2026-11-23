package controller

import (
	"context"
	"fmt"
	"io"
	"math"
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
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// expectedDefaultImageHold prices a default GPT Image reservation from literal
// calculator token counts, independent of the production estimator. Parameters:
// outputTokens is OpenAI's per-image calculator count, outputUSD and inputUSD
// are USD per million tokens, promptBytes bounds the prompt's text tokens, and
// count is the billed image count. Returns: the expected reserved quota.
func expectedDefaultImageHold(outputTokens int, outputUSD, inputUSD float64, promptBytes, count int) int64 {
	perImage := float64(outputTokens)*outputUSD/1e6 + float64(promptBytes)*inputUSD/1e6/float64(count)
	return int64(math.Ceil(perImage*billingratio.QuotaPerUsd)) * int64(count)
}

// TestStrictGPTImageDefaultReservationRelay drives RelayImageHelper against a
// fake OpenAI upstream on a channel with no image tariff, and checks that the
// request is admitted with the calculator-derived hold, that the hold is taken
// before the provider call, and that settlement follows the receipt (or keeps
// the hold when the receipt is unusable). Parameters: t runs the cases.
// Returns: none.
func TestStrictGPTImageDefaultReservationRelay(t *testing.T) {
	const prompt = "A lighthouse" // 12 bytes bounds the prompt's text tokens.
	const fullUsage = `{"input_tokens":1000,"output_tokens":1000,"total_tokens":2000,"input_tokens_details":{"text_tokens":1000,"image_tokens":0}}`
	for _, tc := range []struct {
		name, model, size, quality, usage, configs, extra string
		channelType, count                                int
		reserved, charged                                 int64
		defaultFallback                                   bool
		// verbatim marks a channel type whose body is forwarded unchanged.
		verbatim bool
	}{
		{
			// Omitted size and quality resolve to auto/auto, so the hold is the
			// calculator maximum: max quality at 2880x2880 (23,719 tokens).
			name: "auto_auto_reserves_calculator_maximum", model: "gpt-image-2.5-sunburst", usage: fullUsage, count: 1,
			reserved: expectedDefaultImageHold(23719, 30, 5, len(prompt), 1),
			charged:  17_500, // (1000*5 + 1000*30) / 1e6 USD at 500,000 quota/USD.
		},
		{
			name: "explicit_low_square_2_6_multiplier", model: "gpt-image-2.6-flare", size: "1024x1024", quality: "low", usage: fullUsage, count: 1,
			reserved: expectedDefaultImageHold(823, 30, 5, len(prompt), 1),
			charged:  17_500,
		},
		{
			name: "two_xhigh_landscape_images", model: "gpt-image-2.5-flare", size: "1536x1024", quality: "xhigh", usage: fullUsage, count: 2,
			reserved: expectedDefaultImageHold(2459, 30, 5, len(prompt), 2),
			charged:  17_500,
		},
		{
			name: "missing_usage_keeps_default_hold", model: "gpt-image-2.5-flare-2026-09-08", size: "1024x1024", quality: "max", usage: `null`, count: 1,
			reserved:        expectedDefaultImageHold(7024, 30, 5, len(prompt), 1),
			charged:         expectedDefaultImageHold(7024, 30, 5, len(prompt), 1),
			defaultFallback: true,
		},
		{
			// A token-only channel override (ratio 5 = $10/M input, completion
			// ratio 4 = $40/M output) prices both the hold and the settlement.
			name: "channel_token_override_prices_hold", model: "gpt-image-2.6-sunburst", size: "1024x1024", quality: "high", usage: fullUsage, count: 1,
			configs:  `{"gpt-image-2.6-sunburst":{"ratio":5,"completion_ratio":4}}`,
			reserved: expectedDefaultImageHold(1756, 40, 10, len(prompt), 1),
			charged:  25_000, // (1000*10 + 1000*40) / 1e6 USD.
		},
		{
			// A native generation is re-encoded with the channel's default
			// size, so the hold prices the size actually sent.
			name: "reencoded_generation_reserves_sent_default_size", model: "gpt-image-2.5-flare", quality: "high", usage: fullUsage, count: 1,
			configs:  `{"gpt-image-2.5-flare":{"image":{"default_size":"1024x1024"}}}`,
			reserved: expectedDefaultImageHold(1756, 30, 5, len(prompt), 1),
			charged:  17_500,
		},
		{
			// A verbatim body leaves an omitted size and quality to the
			// provider (auto), whatever defaults the channel configures.
			name: "verbatim_omitted_size_and_quality_reserve_auto", model: "gpt-image-2.5-flare", usage: fullUsage, count: 1,
			channelType: channeltype.OpenAICompatible, verbatim: true,
			configs:  `{"gpt-image-2.5-flare":{"image":{"default_size":"1024x1024","default_quality":"low"}}}`,
			reserved: expectedDefaultImageHold(23719, 30, 5, len(prompt), 1),
			charged:  17_500,
		},
		{
			// A verbatim body renders the client's unclamped n and streams
			// partial images at 100 output tokens each.
			name: "verbatim_unclamped_n_and_partial_images", model: "gpt-image-2.5-flare", size: "1024x1024", quality: "high", usage: fullUsage, count: 12,
			channelType: channeltype.OpenAICompatible, verbatim: true, extra: `"partial_images":2`,
			reserved: expectedDefaultImageHold(1756+200, 30, 5, len(prompt), 12),
			charged:  17_500,
		},
		{
			// Tier multipliers scale a configured per-image price; they never
			// discount the derived default, which already prices quality.
			name: "quality_multiplier_does_not_discount_default", model: "gpt-image-2.5-flare", size: "1024x1024", quality: "high", usage: `{}`, count: 1,
			configs:         `{"gpt-image-2.5-flare":{"image":{"quality_multipliers":{"high":0.2}}}}`,
			reserved:        expectedDefaultImageHold(1756, 30, 5, len(prompt), 1),
			charged:         expectedDefaultImageHold(1756, 30, 5, len(prompt), 1),
			defaultFallback: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10_000_000)
			cleanup := setupCacheBillingLogTest(t)
			defer cleanup()
			require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelGroup{}, &model.UserRequestCost{}))
			require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("quota", balance).Error)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("remain_quota", balance).Error)
			oldLog := config.IsLogConsumeEnabled()
			config.SetLogConsumeEnabled(true)
			defer config.SetLogConsumeEnabled(oldLog)
			oldBatch := config.BatchUpdateEnabled
			config.BatchUpdateEnabled = false
			defer func() { config.BatchUpdateEnabled = oldBatch }()

			channelType := tc.channelType
			if channelType == 0 {
				channelType = channeltype.OpenAI
			}
			var calls atomic.Int32
			var atUpstreamUser atomic.Int64
			var upstreamErr atomic.Bool
			var upstreamBody atomic.Value
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if body, err := io.ReadAll(r.Body); err == nil {
					upstreamBody.Store(string(body))
				} else {
					upstreamErr.Store(true)
				}
				var user model.User
				if model.DB.First(&user, 1).Error != nil {
					upstreamErr.Store(true)
				}
				atUpstreamUser.Store(user.Quota)
				w.Header().Set("Content-Type", "application/json")
				if _, err := fmt.Fprintf(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":%s}`, tc.usage); err != nil {
					upstreamErr.Store(true)
				}
			}))
			defer upstream.Close()

			channel := &model.Channel{Id: 1, Type: channelType, Name: "image-default-reservation", Status: model.ChannelStatusEnabled}
			if tc.configs != "" {
				configs := tc.configs
				channel.ModelConfigs = &configs
			}
			require.NoError(t, model.DB.Create(channel).Error)
			if tc.configs != "" {
				parsed := channel.GetModelPriceConfigsWithContext(context.Background())
				require.Contains(t, parsed, tc.model, "the real channel parser must recognize the override")
				if image := parsed[tc.model].Image; image != nil {
					require.Zero(t, image.PricePerImageUsd, "the override must carry no per-image price")
				}
			}

			fields := []string{fmt.Sprintf(`"model":%q`, tc.model), fmt.Sprintf(`"prompt":%q`, prompt), fmt.Sprintf(`"n":%d`, tc.count)}
			if tc.size != "" {
				fields = append(fields, fmt.Sprintf(`"size":%q`, tc.size))
			}
			if tc.quality != "" {
				fields = append(fields, fmt.Sprintf(`"quality":%q`, tc.quality))
			}
			if tc.extra != "" {
				fields = append(fields, tc.extra)
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader("{"+strings.Join(fields, ",")+"}"))
			c.Request.Header.Set("Content-Type", "application/json")
			gmw.SetLogger(c, logger.Logger)
			requestID := "image-default-reservation-" + tc.name
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
				Mode: mode, ChannelType: channelType, APIType: channeltype.ToAPIType(channelType),
				ChannelId: 1, UserId: 1, TokenId: 1, TokenName: "relay-cache-log-token",
				BaseURL: upstream.URL, APIKey: "test-only", OriginModelName: tc.model,
				ActualModelName: tc.model, RequestURLPath: "/v1/images/generations", StartTime: time.Now(),
			})

			gotErr := RelayImageHelper(c, mode)
			require.Nil(t, gotErr, "an unconfigured channel must be admitted with the default reservation")
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, int32(1), calls.Load())
			require.False(t, upstreamErr.Load())
			require.Equal(t, balance-tc.reserved, atUpstreamUser.Load(), "the default hold is reserved before the provider call")
			sent, _ := upstreamBody.Load().(string)
			if tc.verbatim {
				// Discriminate the forwarding path: the client's body arrives
				// unchanged, without the channel defaults or the clamped n.
				require.NotContains(t, sent, `"quality":"low"`, "body=%s", sent)
				require.NotContains(t, sent, `"size":"1024x1024","quality":"auto"`, "body=%s", sent)
				require.Contains(t, sent, fmt.Sprintf(`"n":%d`, tc.count), "body=%s", sent)
			} else {
				require.Contains(t, sent, `"size":`, "a re-encoded body carries the normalized size: %s", sent)
			}

			var cost model.UserRequestCost
			require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&cost).Error)
			require.Equal(t, tc.charged, cost.Quota)
			var entry model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).First(&entry).Error)
			require.Equal(t, int(tc.charged), entry.Quota)
			if tc.defaultFallback {
				require.Contains(t, entry.Content, "billing_source=default_fallback")
				require.Equal(t, strictImageDefaultFallbackReason, entry.Metadata["billing_estimate_reason"])
			} else {
				require.Contains(t, entry.Content, "billing_source=provider_tokens")
				require.NotContains(t, entry.Metadata, "billing_estimate_reason", "a complete receipt is an exact charge")
			}
			var user model.User
			require.NoError(t, model.DB.First(&user, 1).Error)
			require.Equal(t, balance-tc.charged, user.Quota)
		})
	}
}
