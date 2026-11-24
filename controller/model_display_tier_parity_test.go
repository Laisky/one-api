package controller

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// tierUsage is one usage point at which a displayed tier row is quoted against billing.
type tierUsage struct {
	prompt, cached, completion int
}

// displayedTierQuota quotes usage with the USD-per-1M prices of one displayed row.
// Parameters: input, output and cached are display prices (cached falls back to
// input when omitted, as billing does) and u is the usage. Returns: the quota.
func displayedTierQuota(input, output, cached float64, u tierUsage) int64 {
	if cached <= 0 {
		cached = input
	}
	cost := float64(u.prompt-u.cached)*input + float64(u.cached)*cached + float64(u.completion)*output
	return int64(math.Ceil(cost * ratio.MilliTokensUsd))
}

// billedTierQuota settles usage through request-time billing for one channel model.
// Parameters: t is the test handle, ch is the channel, name the model, provider the
// pricing adaptor and u the usage. Returns: the billed quota.
func billedTierQuota(t *testing.T, ch *model.Channel, name string, provider adaptorpkg.Adaptor, u tierUsage) int64 {
	t.Helper()
	configs := ch.GetModelPriceConfigs()
	at := time.Now()
	modelRatio := pricing.ResolveModelRatioAt(name, configs, ch.GetModelRatioFromConfigs(), provider, at)
	usage := &relaymodel.Usage{PromptTokens: u.prompt, CompletionTokens: u.completion}
	if u.cached > 0 {
		usage.PromptTokensDetails = &relaymodel.UsagePromptTokensDetails{CachedTokens: u.cached}
	}
	return quota.Compute(quota.ComputeInput{
		Usage: usage, ModelName: name, ModelRatio: modelRatio,
		ChannelModelRatio: ch.GetModelRatioFromConfigs(), ChannelCompletionRatio: ch.GetCompletionRatioFromConfigs(),
		GroupRatio: 1, ChannelModelConfigs: configs, PricingAdaptor: provider, RequestTime: at,
	}).TotalQuota
}

// requireTierRowsQuoteBilling asserts every displayed row quotes billing at its entry point.
// Parameters: t is the test handle, rows are the displayed tiers, and the rest
// identify the billed channel model. Returns: nothing.
func requireTierRowsQuoteBilling(t *testing.T, rows []ModelDisplayTier, ch *model.Channel, name string, provider adaptorpkg.Adaptor) {
	t.Helper()
	for _, row := range rows {
		u := tierUsage{prompt: row.InputTokenThreshold, cached: row.InputTokenThreshold / 4, completion: max(1000, row.OutputTokenThreshold)}
		require.Equal(t, billedTierQuota(t, ch, name, provider, u),
			displayedTierQuota(row.InputPrice, row.OutputPrice, row.CachedInputPrice, u),
			"tier row at %d input tokens must quote billing", row.InputTokenThreshold)
	}
}

// TestGetModelsDisplayLocalTiersApplyCumulatively verifies channel-local tiers
// are displayed in billing order with billing's cumulative inheritance: a tier
// that omits ratio, completion_ratio or cached_input_ratio keeps the value
// billing carries from the base or an earlier applicable tier.
func TestGetModelsDisplayLocalTiersApplyCumulatively(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	const name = "gpt-4o"
	m := ratio.MilliTokensUsd
	// The channel omits the base ratio (gpt-4o) or both base scalars
	// (gpt-4o-mini); billing then falls back to the provider default for them.
	local := map[string]model.ModelConfigLocal{
		name: {
			CompletionRatio:  2,
			CachedInputRatio: 0.5 * m,
			Tiers: []model.ModelRatioTierLocal{
				{Ratio: 5 * m, InputTokenThreshold: 5000},
				{Ratio: 3 * m, CompletionRatio: 4, InputTokenThreshold: 1000},
				{CompletionRatio: 6, InputTokenThreshold: 9000},
				{CompletionRatio: 3, InputTokenThreshold: 500},
			},
		},
		"gpt-4o-mini": {Tiers: []model.ModelRatioTierLocal{{Ratio: 3 * m, InputTokenThreshold: 1000}}},
	}
	ch := createDisplayChannel(t, "local-cumulative-tiers", channeltype.OpenAI, name+",gpt-4o-mini", "public", local)
	provider := displayProvider(t, ch)
	providerRatio := provider.GetDefaultModelPricing()[name].Ratio
	require.Positive(t, providerRatio)

	models := fetchModelsDisplay(t, 0, "").Data[channeltype.IdToName(ch.Type)+":"+ch.Name].Models
	info := models[name]
	require.Len(t, info.Tiers, 4, "channel tiers must replace the provider's")
	thresholds := make([]int, 0, len(info.Tiers))
	for _, row := range info.Tiers {
		thresholds = append(thresholds, row.InputTokenThreshold)
	}
	require.Equal(t, []int{500, 1000, 5000, 9000}, thresholds, "rows follow billing's tier order")
	require.InDelta(t, convertDisplayRatio(providerRatio), info.Tiers[0].InputPrice, 1e-12, "an omitted base ratio falls back to the provider's")
	require.InDelta(t, 5.0*4, info.Tiers[2].OutputPrice, 1e-12, "a tier without completion_ratio keeps the earlier tier's")
	require.InDelta(t, 5.0, info.Tiers[3].InputPrice, 1e-12, "a tier without ratio keeps the earlier tier's")
	require.InDelta(t, 0.5, info.Tiers[3].CachedInputPrice, 1e-12, "a tier without cached_input_ratio keeps the base's")
	requireTierRowsQuoteBilling(t, info.Tiers, ch, name, provider)

	mini := models["gpt-4o-mini"]
	require.Len(t, mini.Tiers, 1)
	require.Positive(t, mini.Tiers[0].OutputPrice, "an omitted completion_ratio falls back to the provider's")
	requireTierRowsQuoteBilling(t, mini.Tiers, ch, "gpt-4o-mini", provider)
}

// TestGetModelsDisplayWindowTiersMatchBilling verifies windowed tiers: a window
// without its own tiers reprices the base tiers at the window's scalars, and a
// window's own tiers inherit the window's ratio and completion ratio.
func TestGetModelsDisplayWindowTiersMatchBilling(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	m := ratio.MilliTokensUsd
	always := []model.ClockRangeLocal{{Start: "00:00", End: "00:00"}}
	local := map[string]model.ModelConfigLocal{
		"gpt-4o": {
			Ratio: 2 * m, CompletionRatio: 2,
			Tiers: []model.ModelRatioTierLocal{
				{Ratio: 3 * m, InputTokenThreshold: 1000},
				{Ratio: 5 * m, CompletionRatio: 4, InputTokenThreshold: 5000},
			},
			TimeWindows: []model.TimeWindowLocal{{Name: "inherits-base-tiers", TimeZone: "UTC", Ranges: always, Overlay: model.ModelConfigLocal{CompletionRatio: 5}}},
		},
		"gpt-4o-mini": {
			Ratio: 2 * m, CompletionRatio: 2,
			TimeWindows: []model.TimeWindowLocal{{Name: "own-tiers", TimeZone: "UTC", Ranges: always, Overlay: model.ModelConfigLocal{
				Ratio: 4 * m,
				Tiers: []model.ModelRatioTierLocal{
					{CompletionRatio: 3, InputTokenThreshold: 1000},
					{Ratio: 6 * m, InputTokenThreshold: 5000},
				},
			}}},
		},
	}
	ch := createDisplayChannel(t, "window-tier-parity", channeltype.OpenAI, "gpt-4o,gpt-4o-mini", "public", local)
	provider := displayProvider(t, ch)
	models := fetchModelsDisplay(t, 0, "").Data[channeltype.IdToName(ch.Type)+":"+ch.Name].Models

	inherited := models["gpt-4o"].TimeWindows
	require.Len(t, inherited, 1)
	require.Len(t, inherited[0].Overlay.Tiers, 2, "an untiered window still bills the base tiers")
	require.InDelta(t, 3.0*5, inherited[0].Overlay.Tiers[0].OutputPrice, 1e-12, "base tier priced at the window completion ratio")
	requireTierRowsQuoteBilling(t, inherited[0].Overlay.Tiers, ch, "gpt-4o", provider)

	own := models["gpt-4o-mini"].TimeWindows
	require.Len(t, own, 1)
	require.Len(t, own[0].Overlay.Tiers, 2)
	require.InDelta(t, 4.0, own[0].Overlay.Tiers[0].InputPrice, 1e-12, "a windowed tier without ratio keeps the window's")
	require.InDelta(t, 6.0*3, own[0].Overlay.Tiers[1].OutputPrice, 1e-12, "cumulative completion ratio inside the window")
	requireTierRowsQuoteBilling(t, own[0].Overlay.Tiers, ch, "gpt-4o-mini", provider)
}

// TestGetModelsDisplayProviderTiersInheritBaseCache verifies provider tiers that
// omit cached_input_ratio display the base cache price billing charges.
func TestGetModelsDisplayProviderTiersInheritBaseCache(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	const name = "qwen3-max"
	ch := createDisplayChannel(t, "provider-tier-cache", channeltype.Ali, name, "public", nil)
	provider := displayProvider(t, ch)
	cfg, ok := provider.GetDefaultModelPricing()[name]
	require.True(t, ok)
	require.Positive(t, cfg.CachedInputRatio)
	require.NotEmpty(t, cfg.Tiers)
	require.Zero(t, cfg.Tiers[0].CachedInputRatio, "fixture needs a tier that omits the cache price")

	info := fetchModelsDisplay(t, 0, "").Data[channeltype.IdToName(ch.Type)+":"+ch.Name].Models[name]
	require.Len(t, info.Tiers, len(cfg.Tiers))
	for i, row := range info.Tiers {
		eff := pricing.ResolveEffectivePricingForUsageFromConfig(cfg.Tiers[i].InputTokenThreshold, cfg.Tiers[i].OutputTokenThreshold, cfg)
		require.InDelta(t, convertDisplayRatio(eff.CachedInputRatio), row.CachedInputPrice, 1e-9)
		require.InDelta(t, convertDisplayRatio(eff.InputRatio), row.InputPrice, 1e-9)
		u := tierUsage{prompt: row.InputTokenThreshold, cached: row.InputTokenThreshold / 2, completion: 1000}
		require.InDelta(t, float64(billedTierQuota(t, ch, name, provider, u)),
			float64(displayedTierQuota(row.InputPrice, row.OutputPrice, row.CachedInputPrice, u)), 1,
			"tier row at %d input tokens must quote billing", row.InputTokenThreshold)
	}
}

// TestGetModelsDisplayLocalOverrideReplacesProviderTiers verifies a channel entry
// without tiers drops the provider tiers, because billing selects the channel
// config wholesale.
func TestGetModelsDisplayLocalOverrideReplacesProviderTiers(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	const name = "gpt-5.5-pro"
	ch := createDisplayChannel(t, "provider-tiers-dropped", channeltype.OpenAI, name, "public", map[string]model.ModelConfigLocal{name: {MaxTokens: 4096}})
	provider := displayProvider(t, ch)
	require.NotEmpty(t, provider.GetDefaultModelPricing()[name].Tiers)

	info := fetchModelsDisplay(t, 0, "").Data[channeltype.IdToName(ch.Type)+":"+ch.Name].Models[name]
	require.Empty(t, info.Tiers, "billing ignores provider tiers once the channel configures the model")
	u := tierUsage{prompt: 300_000, completion: 1000}
	require.Equal(t, billedTierQuota(t, ch, name, provider, u), displayedTierQuota(info.InputPrice, info.OutputPrice, info.CachedInputPrice, u))
}
