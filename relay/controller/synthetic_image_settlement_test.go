package controller

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestSyntheticNonGeminiImageSettlement checks a non-Gemini context through the real quota tracker.
func TestSyntheticNonGeminiImageSettlement(t *testing.T) {
	for _, tc := range []struct {
		name       string
		images     int
		imagePrice float64
		want       int64
	}{
		{"image", 1, 0.02, 10005}, {"no_image", 0, 0.02, 5}, {"missing_price", 1, 0, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 20000
			xaiVideoSetup(t, balance, false)
			const name = "synthetic-non-gemini-image"
			cfg := dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, Image: &dbmodel.ImagePricingLocal{PricePerImageUsd: tc.imagePrice, DefaultSize: "1024x1024", DefaultQuality: "standard"}}
			channel := &dbmodel.Channel{}
			require.NoError(t, channel.SetModelPriceConfigs(map[string]dbmodel.ModelConfigLocal{name: cfg}))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.ChannelRatio, 1.0)
			c.Set(ctxkey.OutputImageCount, tc.images)
			c.Set(ctxkey.ChannelModel, channel)
			m := &meta.Meta{ActualModelName: name, ChannelType: channeltype.OpenAI, APIType: apitype.OpenAI}
			usage := &model.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}
			tracker := streaming.NewQuotaTracker(streaming.QuotaTrackerParams{UserID: fallbackUserID, TokenID: fallbackTokenID, ModelName: name, ModelRatio: 1, GroupRatio: 1, Ctx: context.Background(), ChannelModelConfigs: map[string]dbmodel.ModelConfigLocal{name: cfg}})
			applyOutputImageCharges(c, &usage, m)
			result, charged, apiErr := finalizeStreamingUsage(tracker, usage, nil)
			require.Nil(t, apiErr)
			require.Equal(t, tc.want, charged)
			require.Equal(t, balance-tc.want, reloadUserQuota(t))
			require.Equal(t, 2, result.PromptTokens)
			require.Equal(t, 3, result.CompletionTokens)
			require.Zero(t, result.ImageTokensBilledSeparately)
		})
	}
}
