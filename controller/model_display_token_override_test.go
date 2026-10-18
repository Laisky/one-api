package controller

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// TestGetModelsDisplayLocalTokenTariffMatchesBilling checks actual HTTP display
// against request-time billing for saved DeepSeek overrides. Parameters: t is
// the test handle. Returns: nothing; assertions report pricing mismatches.
func TestGetModelsDisplayLocalTokenTariffMatchesBilling(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	type fixture struct {
		ch         *model.Channel
		name, kind string
		local      model.ModelConfigLocal
	}
	var fixtures []fixture
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"} {
		for _, kind := range []string{"flat", "free_cache", "explicit_cache", "cache_only", "own_window"} {
			local := model.ModelConfigLocal{Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 3}
			switch kind {
			case "free_cache":
				local.CachedInputRatio = -1
			case "explicit_cache":
				local.CachedInputRatio = 0.2 * ratio.MilliTokensUsd
			case "cache_only":
				local.Ratio = 0
				local.CachedInputRatio = 0.2 * ratio.MilliTokensUsd
			case "own_window":
				local.TimeWindows = []model.TimeWindowLocal{{Name: "local-always", TimeZone: "UTC", Ranges: []model.ClockRangeLocal{{Start: "00:00", End: "00:00"}}, Overlay: model.ModelConfigLocal{Ratio: 4 * ratio.MilliTokensUsd, CachedInputRatio: 0.4 * ratio.MilliTokensUsd}}}
			}
			ch := &model.Channel{Name: fmt.Sprintf("%s-%s", name, kind), Type: channeltype.DeepSeek, Status: model.ChannelStatusEnabled, Models: name, Group: "public"}
			require.NoError(t, ch.SetModelPriceConfigs(map[string]model.ModelConfigLocal{name: local}))
			require.NoError(t, model.DB.Create(ch).Error)
			fixtures = append(fixtures, fixture{ch: ch, name: name, kind: kind, local: local})
		}
	}
	for _, f := range fixtures {
		t.Run(f.ch.Name, func(t *testing.T) {
			info := fetchPerPageDisplayModel(t, f.ch, f.name, 0).info
			configs := f.ch.GetModelPriceConfigs()
			at := time.Now()
			provider := &deepseek.Adaptor{}
			input := pricing.ResolveModelRatioAt(f.name, configs, f.ch.GetModelRatioFromConfigs(), provider, at)
			resolved, ok := pricing.ResolveModelConfig(f.name, configs, provider, at)
			require.True(t, ok)
			displayedInput, displayedOutput, displayedCache := info.InputPrice, info.OutputPrice, info.CachedInputPrice
			if f.kind == "own_window" {
				require.Len(t, info.TimeWindows, 1)
				require.Equal(t, "local-always", info.ActiveTimeWindow)
				require.Equal(t, "local-always", info.TimeWindows[0].Name)
				displayedInput = info.TimeWindows[0].Overlay.InputPrice
				displayedOutput = info.TimeWindows[0].Overlay.OutputPrice
				displayedCache = info.TimeWindows[0].Overlay.CachedInputPrice
			} else {
				require.Empty(t, info.TimeWindows, "a saved local token tariff must not advertise inherited provider schedules")
				require.Empty(t, info.ActiveTimeWindow)
			}
			expectedCache := resolved.CachedInputRatio
			if expectedCache == 0 {
				expectedCache = input
			}
			if expectedCache < 0 {
				expectedCache = 0
			}
			require.InDelta(t, input/ratio.MilliTokensUsd, displayedInput, 1e-12)
			require.InDelta(t, input*f.local.CompletionRatio/ratio.MilliTokensUsd, displayedOutput, 1e-12)
			require.InDelta(t, expectedCache/ratio.MilliTokensUsd, displayedCache, 1e-12)
			billed := quota.Compute(quota.ComputeInput{Usage: &relaymodel.Usage{PromptTokens: 10000, CompletionTokens: 5000, PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 2000}}, ModelName: f.name, ModelRatio: input, ChannelModelRatio: f.ch.GetModelRatioFromConfigs(), ChannelCompletionRatio: f.ch.GetCompletionRatioFromConfigs(), GroupRatio: 1, ChannelModelConfigs: configs, PricingAdaptor: provider, RequestTime: at})
			displayedQuota := int64(math.Ceil((8000*displayedInput + 2000*displayedCache + 5000*displayedOutput) * ratio.MilliTokensUsd))
			require.Equal(t, displayedQuota, billed.TotalQuota, "displayed token rates must quote actual billing")
		})
	}
}
