package controller

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// fetchModelsDisplay requests /api/models/display through the real handler.
// Parameters: t is the test handle, userID is zero for anonymous or a user id
// otherwise, and rawQuery is the URL query without the leading "?".
// Returns: the decoded successful response.
func fetchModelsDisplay(t *testing.T, userID int, rawQuery string) ModelsDisplayResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/models/display", func(c *gin.Context) {
		if userID != 0 {
			c.Set(ctxkey.Id, userID)
		}
		GetModelsDisplay(c)
	})
	target := "/api/models/display"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response ModelsDisplayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)
	return response
}

// displayProvider returns the initialized pricing adaptor the display handler uses for ch.
// Parameters: t is the test handle and ch is the persisted channel.
// Returns: the adaptor resolved from the channel type.
func displayProvider(t *testing.T, ch *model.Channel) adaptorpkg.Adaptor {
	t.Helper()
	provider := relay.GetAdaptor(channeltype.ToAPIType(ch.Type))
	require.NotNil(t, provider)
	provider.Init(&metalib.Meta{ChannelType: ch.Type})
	return provider
}

// createDisplayChannel persists an enabled channel with local model configs.
// Parameters: t is the test handle, name/channelType/models/group describe the
// channel, and local holds the channel model_configs (nil for none).
// Returns: the persisted channel.
func createDisplayChannel(t *testing.T, name string, channelType int, models, group string, local map[string]model.ModelConfigLocal) *model.Channel {
	t.Helper()
	ch := &model.Channel{Name: name, Type: channelType, Status: model.ChannelStatusEnabled, Models: models, Group: group}
	if local != nil {
		require.NoError(t, ch.SetModelPriceConfigs(local))
	}
	require.NoError(t, model.DB.Create(ch).Error)
	return ch
}

// TestGetModelsDisplayWindowTierInheritsWindowCompletionRatio verifies that a
// tier inside a time window that omits completion_ratio is displayed at the
// window's completion ratio, which is what billing charges.
func TestGetModelsDisplayWindowTierInheritsWindowCompletionRatio(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	const name = "window-tier-model"
	local := map[string]model.ModelConfigLocal{name: {
		Ratio:           2 * ratio.MilliTokensUsd,
		CompletionRatio: 2,
		TimeWindows: []model.TimeWindowLocal{{
			Name:     "always",
			TimeZone: "UTC",
			Ranges:   []model.ClockRangeLocal{{Start: "00:00", End: "00:00"}},
			Overlay: model.ModelConfigLocal{
				CompletionRatio: 5,
				Tiers:           []model.ModelRatioTierLocal{{Ratio: 3 * ratio.MilliTokensUsd, InputTokenThreshold: 1000}},
			},
		}},
	}}
	ch := createDisplayChannel(t, "window-tier", channeltype.OpenAI, name, "public", local)

	at := time.Now()
	resolved, ok := pricing.ResolveModelConfig(name, ch.GetModelPriceConfigs(), displayProvider(t, ch), at)
	require.True(t, ok)
	billed := pricing.ResolveEffectivePricingForUsageFromConfig(1000, 0, resolved)
	require.InDelta(t, 3*ratio.MilliTokensUsd*5, billed.OutputRatio, 1e-12, "billing prices the tier output at the window completion ratio")

	info := fetchModelsDisplay(t, 0, "").Data[channeltype.IdToName(ch.Type)+":"+ch.Name].Models[name]
	require.Len(t, info.TimeWindows, 1)
	overlay := info.TimeWindows[0].Overlay
	require.Len(t, overlay.Tiers, 1)
	require.InDelta(t, billed.InputRatio/ratio.MilliTokensUsd, overlay.Tiers[0].InputPrice, 1e-12)
	require.InDelta(t, billed.OutputRatio/ratio.MilliTokensUsd, overlay.Tiers[0].OutputPrice, 1e-12,
		"the windowed tier must not fall back to the base completion ratio")
	require.InDelta(t, 2*5.0, overlay.OutputPrice, 1e-12)
}

// TestGetModelsDisplayActiveWindowIsFirstMatch verifies active_time_window
// names the first matching window billing applies, even when it is unnamed.
func TestGetModelsDisplayActiveWindowIsFirstMatch(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	const name = "window-first-match-model"
	always := []model.ClockRangeLocal{{Start: "00:00", End: "00:00"}}
	local := map[string]model.ModelConfigLocal{name: {
		Ratio:           2 * ratio.MilliTokensUsd,
		CompletionRatio: 2,
		TimeWindows: []model.TimeWindowLocal{
			{TimeZone: "UTC", Ranges: always, Overlay: model.ModelConfigLocal{Ratio: 4 * ratio.MilliTokensUsd}},
			{Name: "later-named", TimeZone: "UTC", Ranges: always, Overlay: model.ModelConfigLocal{Ratio: 8 * ratio.MilliTokensUsd}},
		},
	}}
	ch := createDisplayChannel(t, "window-first-match", channeltype.OpenAI, name, "public", local)

	resolved, ok := pricing.ResolveModelConfig(name, ch.GetModelPriceConfigs(), displayProvider(t, ch), time.Now())
	require.True(t, ok)
	require.InDelta(t, 4*ratio.MilliTokensUsd, resolved.Ratio, 1e-12, "billing applies the first (unnamed) window")

	info := fetchModelsDisplay(t, 0, "").Data[channeltype.IdToName(ch.Type)+":"+ch.Name].Models[name]
	require.Len(t, info.TimeWindows, 2)
	require.Empty(t, info.ActiveTimeWindow, "a later window that billing never applies must not be reported as active")
}

// TestGetModelsDisplayAppliesChannelContextLength verifies a channel
// context_length override reaches both the min_context_length filter and the
// displayed entry, while models without one keep the adaptor value.
func TestGetModelsDisplayAppliesChannelContextLength(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	local := map[string]model.ModelConfigLocal{
		"gpt-4o":      {MaxTokens: 4096, ContextLength: 1_000_000},
		"gpt-4o-mini": {MaxTokens: 4096},
	}
	ch := createDisplayChannel(t, "context-override", channeltype.OpenAI, "gpt-4o,gpt-4o-mini", "public", local)
	key := channeltype.IdToName(ch.Type) + ":" + ch.Name
	adaptorMini := displayProvider(t, ch).GetDefaultModelPricing()["gpt-4o-mini"].ContextLength
	require.Positive(t, adaptorMini)

	all := fetchModelsDisplay(t, 0, "").Data[key].Models
	require.Equal(t, int32(1_000_000), all["gpt-4o"].ContextLength)
	require.Equal(t, adaptorMini, all["gpt-4o-mini"].ContextLength)

	filtered := fetchModelsDisplay(t, 0, "min_context_length=500000").Data[key].Models
	require.Contains(t, filtered, "gpt-4o", "the configured context window must satisfy the filter")
	require.NotContains(t, filtered, "gpt-4o-mini")
}

// TestGetModelsDisplayVideoPricingUsesChannelOverride verifies top-level
// video_pricing follows billing's precedence: the channel video block, then the
// adaptor default.
func TestGetModelsDisplayVideoPricingUsesChannelOverride(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	local := map[string]model.ModelConfigLocal{
		"sora-2":             {Video: &model.VideoPricingLocal{PerSecondUsd: 0.25, BaseResolution: "1280x720"}},
		"custom-video-model": {Video: &model.VideoPricingLocal{PerSecondUsd: 0.4, InputImageUsd: 0.01, BaseResolution: "1920x1080", ResolutionMultipliers: map[string]float64{"1920x1080": 1, "3840x2160": 2}}},
		"sora-2-pro":         {MaxTokens: 1},
	}
	ch := createDisplayChannel(t, "video-override", channeltype.OpenAI, "sora-2,sora-2-pro,custom-video-model", "public", local)
	provider := displayProvider(t, ch)
	models := fetchModelsDisplay(t, 0, "").Data[channeltype.IdToName(ch.Type)+":"+ch.Name].Models

	for _, name := range []string{"sora-2", "custom-video-model", "sora-2-pro"} {
		resolved, ok := pricing.ResolveModelConfig(name, ch.GetModelPriceConfigs(), provider, time.Now())
		require.True(t, ok)
		expected := resolved.Video
		if expected == nil || !expected.HasData() {
			// Billing falls back to the provider default when the channel has no video block.
			fallback, ok := pricing.ResolveModelConfig(name, nil, provider, time.Now())
			require.True(t, ok)
			expected = fallback.Video
		}
		require.NotNil(t, expected, name)
		require.Equal(t, buildVideoDisplayPricing(expected), models[name].VideoPricing, name)
	}
	require.InDelta(t, 0.25, models["sora-2"].VideoPricing.PerSecondUsd, 1e-12)
	require.InDelta(t, 0.4, models["custom-video-model"].VideoPricing.PerSecondUsd, 1e-12)
	require.InDelta(t, 0.30, models["sora-2-pro"].VideoPricing.PerSecondUsd, 1e-12)
}

// TestGetModelsDisplayEmbeddingPricingUsesChannelOverride verifies top-level
// embedding_pricing shows the channel embedding block the pricing resolver
// selects, falling back to the adaptor default only when the channel has none.
func TestGetModelsDisplayEmbeddingPricingUsesChannelOverride(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	local := map[string]model.ModelConfigLocal{
		"text-embedding-v4": {Embedding: &model.EmbeddingPricingLocal{TextTokenRatio: 4 * ratio.MilliTokensUsd}},
		"custom-embedding":  {Ratio: 1 * ratio.MilliTokensUsd, Embedding: &model.EmbeddingPricingLocal{TextTokenRatio: 1 * ratio.MilliTokensUsd, ImageTokenRatio: 6 * ratio.MilliTokensUsd}},
		"text-embedding-v3": {MaxTokens: 1},
	}
	ch := createDisplayChannel(t, "embedding-override", channeltype.AliBailian, "text-embedding-v3,text-embedding-v4,custom-embedding", "public", local)
	provider := displayProvider(t, ch)
	models := fetchModelsDisplay(t, 0, "").Data[channeltype.IdToName(ch.Type)+":"+ch.Name].Models
	convert := func(r float64) float64 { return r / ratio.MilliTokensUsd }

	for _, name := range []string{"text-embedding-v4", "custom-embedding"} {
		resolved, ok := pricing.ResolveModelConfig(name, ch.GetModelPriceConfigs(), provider, time.Now())
		require.True(t, ok)
		require.NotNil(t, resolved.Embedding, name)
		require.Equal(t, buildEmbeddingDisplayPricing(resolved.Embedding, convertDisplayRatio), models[name].EmbeddingPricing, name)
	}
	require.InDelta(t, convert(4*ratio.MilliTokensUsd), models["text-embedding-v4"].EmbeddingPricing.TextTokenPrice, 1e-12)
	require.InDelta(t, convert(6*ratio.MilliTokensUsd), models["custom-embedding"].EmbeddingPricing.ImageTokenPrice, 1e-12)

	// The displayed modality prices must quote what token billing settles.
	for _, name := range []string{"text-embedding-v4", "custom-embedding"} {
		configs := ch.GetModelPriceConfigs()
		at := time.Now()
		details := &relaymodel.UsagePromptTokensDetails{TextTokens: 1000, ImageTokens: 1000}
		billed := quota.Compute(quota.ComputeInput{
			Usage:     &relaymodel.Usage{PromptTokens: 2000, PromptTokensDetails: details},
			ModelName: name, ModelRatio: pricing.ResolveModelRatioAt(name, configs, ch.GetModelRatioFromConfigs(), provider, at),
			ChannelModelRatio: ch.GetModelRatioFromConfigs(), ChannelCompletionRatio: ch.GetCompletionRatioFromConfigs(),
			GroupRatio: 1, ChannelModelConfigs: configs, PricingAdaptor: provider, RequestTime: at,
		})
		shown := models[name].EmbeddingPricing
		imagePrice := shown.ImageTokenPrice
		if imagePrice == 0 {
			imagePrice = shown.TextTokenPrice
		}
		displayed := int64(math.Ceil((1000*shown.TextTokenPrice + 1000*imagePrice) * ratio.MilliTokensUsd))
		require.Equal(t, billed.TotalQuota, displayed, "%s embedding display must quote billing", name)
	}

	adaptorV3 := provider.GetDefaultModelPricing()["text-embedding-v3"].Embedding
	require.NotNil(t, adaptorV3)
	require.Equal(t, buildEmbeddingDisplayPricing(adaptorV3, convertDisplayRatio), models["text-embedding-v3"].EmbeddingPricing,
		"a channel config without an embedding block keeps the adaptor display")
}

// convertDisplayRatio mirrors the display handler's quota-ratio to USD-per-1M conversion.
// Parameters: r is a quota ratio per token. Returns: USD per million tokens.
func convertDisplayRatio(r float64) float64 {
	if r <= 0 {
		return 0
	}
	if r < 0.001 {
		return r * 1_000_000
	}
	return (r * 1_000_000) / ratio.QuotaPerUsd
}

// TestGetModelsDisplayKeepsSameNamedChannels verifies channels sharing a type
// and name all survive in the anonymous and logged-in responses: the lowest id
// keeps the legacy "type:name" key, later collisions get ordinal suffixes in
// ascending id order, a real channel name is never displaced by a suffix, and
// no channel UUID or integer id is exposed in the keys.
func TestGetModelsDisplayKeepsSameNamedChannels(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	group := fmt.Sprintf("dup-display-%d", time.Now().UnixNano())
	// Creation order fixes ascending ids: a real "dup-channel#2" exists before the
	// second "dup-channel", and a real "dup-channel#3" after it.
	fixtures := []struct{ name, model, wantKeySuffix string }{
		{name: "dup-channel", model: "gpt-4o", wantKeySuffix: ""},
		{name: "dup-channel#2", model: "gpt-4.1", wantKeySuffix: "#2"},
		{name: "dup-channel", model: "gpt-4o-mini", wantKeySuffix: "#4"},
		{name: "dup-channel#3", model: "gpt-4.1-mini", wantKeySuffix: "#3"},
	}
	user := &model.User{Username: "dup-display", Password: "fixture", Group: group, Status: model.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	channels := make([]*model.Channel, 0, len(fixtures))
	for _, f := range fixtures {
		ch := createDisplayChannel(t, f.name, channeltype.OpenAI, f.model, group, nil)
		require.NoError(t, model.DB.Create(&model.Ability{Group: group, Model: f.model, ChannelId: ch.Id, Enabled: true}).Error)
		if len(channels) > 0 {
			require.Less(t, channels[len(channels)-1].Id, ch.Id)
		}
		channels = append(channels, ch)
	}

	base := channeltype.IdToName(channeltype.OpenAI) + ":dup-channel"
	for _, userID := range []int{0, user.Id} {
		t.Run(fmt.Sprintf("user_%d", userID), func(t *testing.T) {
			for range 3 {
				data := fetchModelsDisplay(t, userID, "").Data
				require.Len(t, data, len(fixtures), "no same-named channel may be dropped")
				for i, f := range fixtures {
					key := base + f.wantKeySuffix
					require.Contains(t, data, key)
					require.Equal(t, key, data[key].ChannelName)
					require.Len(t, data[key].Models, 1)
					require.Contains(t, data[key].Models, f.model, "channel %d owns key %s", i, key)
					require.NotContains(t, key, channels[i].UUID)
				}
			}
		})
	}
}
