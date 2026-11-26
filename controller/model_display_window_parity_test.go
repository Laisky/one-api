package controller

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

// windowParityInstants returns the display instants the window parity sweep
// prices: every three hours across one week in UTC, which crosses every
// built-in time-of-day window, plus dates on both sides of dated schedules.
func windowParityInstants() []time.Time {
	start := time.Date(2026, time.November, 23, 0, 30, 0, 0, time.UTC) // Monday
	instants := make([]time.Time, 0, 64)
	for at := start; at.Before(start.AddDate(0, 0, 7)); at = at.Add(3 * time.Hour) {
		instants = append(instants, at)
	}
	return append(instants,
		time.Date(2025, time.June, 2, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.November, 30, 23, 59, 0, 0, time.UTC),
		time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, time.March, 3, 12, 0, 0, 0, time.UTC),
	)
}

// windowedTokenModels lists every provider-default model that carries time
// windows and bills by token only, per channel type, deduplicating shared tables.
// Parameters: t is the test handle. Returns: channel type to sorted model names
// and the matching pricing adaptors.
func windowedTokenModels(t *testing.T) (map[int][]string, map[int]adaptorpkg.Adaptor) {
	t.Helper()
	models := map[int][]string{}
	providers := map[int]adaptorpkg.Adaptor{}
	seen := map[string]struct{}{}
	for channelType := 1; channelType < channeltype.Dummy; channelType++ {
		provider := relay.GetAdaptor(channeltype.ToAPIType(channelType))
		if provider == nil {
			continue
		}
		if aware, ok := provider.(adaptorpkg.ChannelTypeAware); ok {
			aware.SetChannelType(channelType)
		}
		for name, cfg := range provider.GetDefaultModelPricing() {
			// Media and unit tariffs settle through their own relays, so the
			// token settlement below is the reference only for token models.
			media := cfg.Audio != nil || cfg.Video != nil || cfg.Image != nil || cfg.Embedding != nil ||
				cfg.PerCall != nil || cfg.PerPage != nil
			if len(cfg.TimeWindows) == 0 || cfg.Ratio <= 0 || media || adaptorpkg.RequiresOperatorTariff(provider, name) {
				continue
			}
			key := provider.GetChannelName() + "/" + name
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			models[channelType] = append(models[channelType], name)
			providers[channelType] = provider
		}
		sort.Strings(models[channelType])
	}
	return models, providers
}

// TestModelDisplayTopLevelQuotesBillingAcrossWindows sweeps every built-in
// token model with time windows (DeepSeek peaks, Beijing peaks, dated schedules
// such as GPT-5.6 Sol) across a week and both sides of dated cutoffs. At every
// instant the top-level display prices, and every displayed tier row, must
// settle exactly what request-time billing charges at that instant, for the
// provider default and for a metadata-only channel entry that inherits the
// provider schedule. inactive_base_pricing must appear exactly while a window
// applies. Parameter t reports the first display that disagrees with billing.
func TestModelDisplayTopLevelQuotesBillingAcrossWindows(t *testing.T) {
	models, providers := windowedTokenModels(t)
	total := 0
	for channelType, names := range models {
		provider := providers[channelType]
		for _, name := range names {
			total++
			bare := &model.Channel{Type: channelType, Name: "window-parity"}
			inheriting := &model.Channel{Type: channelType, Name: "window-parity-inherit"}
			require.NoError(t, inheriting.SetModelPriceConfigs(map[string]model.ModelConfigLocal{name: {MaxTokens: 4096}}))
			entry := inheriting.GetModelPriceConfigs()[name]
			for _, at := range windowParityInstants() {
				dc := modelDisplayContext{
					channel:        bare,
					adaptor:        provider,
					defaultPricing: provider.GetDefaultModelPricing(),
					convert:        convertDisplayRatio,
					now:            at,
				}
				activeName := pricing.ActiveTimeWindowName(provider.GetDefaultModelPricing()[name], at)
				windowActive := false
				for _, window := range provider.GetDefaultModelPricing()[name].TimeWindows {
					if pricing.MatchTimeWindow(window, at) {
						windowActive = true
						break
					}
				}
				for _, tc := range []struct {
					label    string
					ch       *model.Channel
					override *model.ModelConfigLocal
				}{
					{"provider default", bare, nil},
					{"metadata-only entry", inheriting, &entry},
				} {
					label := fmt.Sprintf("%s %s/%s at %s", tc.label, provider.GetChannelName(), name, at.Format(time.RFC3339))
					dc.channel = tc.ch
					info := dc.buildModelDisplay(name, tc.override)
					require.Equal(t, activeName, info.ActiveTimeWindow, label)
					require.Equal(t, windowActive, info.InactiveBasePricing != nil, label)
					for i := range info.TimeWindows {
						require.NotNil(t, info.TimeWindows[i].EffectivePricing, label)
					}
					u := tierUsage{prompt: 10_000, cached: 2_500, completion: 1_000}
					require.Equal(t, billedTierQuotaAt(t, tc.ch, name, provider, u, at),
						displayedTierQuota(info.InputPrice, info.OutputPrice, info.CachedInputPrice, u),
						"top-level prices must settle billing: "+label)
					for _, row := range info.Tiers {
						u := tierUsage{prompt: row.InputTokenThreshold, cached: row.InputTokenThreshold / 4, completion: max(1000, row.OutputTokenThreshold)}
						require.Equal(t, billedTierQuotaAt(t, tc.ch, name, provider, u, at),
							displayedTierQuota(row.InputPrice, row.OutputPrice, row.CachedInputPrice, u),
							"tier row at %d input tokens must settle billing: %s", row.InputTokenThreshold, label)
					}
				}
			}
		}
	}
	require.Positive(t, total, "control: the catalogs must contain windowed token models")
}
