package controller

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

// pinModelsDisplayClock makes the public price surfaces quote tariffs at at
// until the test ends. Parameters: t is the test handle and at the instant.
func pinModelsDisplayClock(t *testing.T, at time.Time) {
	t.Helper()
	previous := modelsDisplayNow
	modelsDisplayNow = func() time.Time { return at }
	t.Cleanup(func() { modelsDisplayNow = previous })
}

// solDisplayTariff is one GPT-5.6 Sol display tariff in USD per million tokens.
type solDisplayTariff struct {
	input, cached, write, output              float64
	longInput, longCached, longWrite, longOut float64
}

var (
	solDisplayPromotional = solDisplayTariff{4, .40, 5, 20, 8, .80, 10, 30}
	solDisplayStandard    = solDisplayTariff{5, .50, 6.25, 30, 10, 1, 12.5, 45}
)

// requireSolDisplayTariff checks the top-level token prices and the long-context tier.
// Parameters: t is the test handle, info the rendered model, want the expected
// tariff and label the failure context.
func requireSolDisplayTariff(t *testing.T, info ModelDisplayInfo, want solDisplayTariff, label string) {
	t.Helper()
	require.InDelta(t, want.input, info.InputPrice, 1e-9, label)
	require.InDelta(t, want.cached, info.CachedInputPrice, 1e-9, label)
	require.InDelta(t, want.write, info.CacheWrite5mPrice, 1e-9, label)
	require.Zero(t, info.CacheWrite1hPrice, label)
	require.InDelta(t, want.output, info.OutputPrice, 1e-9, label)
	require.Len(t, info.Tiers, 1, label)
	tier := info.Tiers[0]
	require.Equal(t, 272_001, tier.InputTokenThreshold, label)
	require.InDelta(t, want.longInput, tier.InputPrice, 1e-9, label)
	require.InDelta(t, want.longCached, tier.CachedInputPrice, 1e-9, label)
	require.InDelta(t, want.longWrite, tier.CacheWrite5mPrice, 1e-9, label)
	require.InDelta(t, want.longOut, tier.OutputPrice, 1e-9, label)
}

// TestGetModelsDisplayQuotesGPT56SolEffectiveTariffAcrossCutoff renders the
// public model display through the real handler on both sides of the native
// GPT-5.6 Sol cutoff. Before the cutoff it must quote the promotional price;
// from the cutoff on it must quote the standard price billing charges,
// including cached input, cache writes and the long-context tier, for both
// aliases, anonymously (cached) and authenticated. An explicit channel tariff
// for the same model must stay exactly as configured. Parameter t reports any
// display that disagrees with billing.
func TestGetModelsDisplayQuotesGPT56SolEffectiveTariffAcrossCutoff(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	cutoff := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	const models = "gpt-5.6-sol,gpt-5.6"
	native := createDisplayChannel(t, "sol-native", channeltype.OpenAI, models, "default", nil)
	operator := createDisplayChannel(t, "sol-operator", channeltype.OpenAI, models, "default", map[string]model.ModelConfigLocal{
		"gpt-5.6-sol": {
			Ratio: 7 * ratio.MilliTokensUsd, CompletionRatio: 3,
			CachedInputRatio: 0.7 * ratio.MilliTokensUsd, CacheWrite5mRatio: 8 * ratio.MilliTokensUsd,
		},
	})
	user := &model.User{Username: "sol-display", Password: "fixture", Group: "default", Status: model.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	for _, ch := range []*model.Channel{native, operator} {
		for _, name := range []string{"gpt-5.6-sol", "gpt-5.6"} {
			require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: name, ChannelId: ch.Id, Enabled: true}).Error)
		}
	}
	nativeKey := channeltype.IdToName(native.Type) + ":" + native.Name
	operatorKey := channeltype.IdToName(operator.Type) + ":" + operator.Name
	provider := displayProvider(t, native)

	for _, at := range []time.Time{cutoff.Add(-time.Nanosecond), cutoff, cutoff.AddDate(0, 0, 1)} {
		pinModelsDisplayClock(t, at)
		want, standard := solDisplayPromotional, !at.Before(cutoff)
		if standard {
			want = solDisplayStandard
		}
		for _, userID := range []int{0, user.Id} {
			response := fetchModelsDisplay(t, userID, "")
			for _, name := range []string{"gpt-5.6-sol", "gpt-5.6"} {
				label := fmt.Sprintf("%s at %s user=%d", name, at.Format(time.RFC3339Nano), userID)
				info, ok := response.Data[nativeKey].Models[name]
				require.True(t, ok, label)
				requireSolDisplayTariff(t, info, want, "native "+label)

				// Parity with the billing resolver at the same instant.
				billed, found := pricing.ResolveModelConfig(name, nil, provider, at)
				require.True(t, found, label)
				short := pricing.ResolveEffectivePricingForUsageFromConfig(272_000, 0, billed)
				require.InDelta(t, short.InputRatio/ratio.MilliTokensUsd, info.InputPrice, 1e-9, label)
				require.InDelta(t, short.OutputRatio/ratio.MilliTokensUsd, info.OutputPrice, 1e-9, label)
				require.InDelta(t, short.CachedInputRatio/ratio.MilliTokensUsd, info.CachedInputPrice, 1e-9, label)
				require.InDelta(t, short.CacheWrite5mRatio/ratio.MilliTokensUsd, info.CacheWrite5mPrice, 1e-9, label)
				long := pricing.ResolveEffectivePricingForUsageFromConfig(272_001, 0, billed)
				require.InDelta(t, long.InputRatio/ratio.MilliTokensUsd, info.Tiers[0].InputPrice, 1e-9, label)
				require.InDelta(t, long.OutputRatio/ratio.MilliTokensUsd, info.Tiers[0].OutputPrice, 1e-9, label)

				// The schedule metadata stays published on both sides of the cutoff.
				require.Len(t, info.TimeWindows, 1, label)
				window := info.TimeWindows[0]
				require.Equal(t, "gpt-5.6-sol-standard-from-december-2026", window.Name, label)
				require.Equal(t, "2026-12-01", window.DateFrom, label)
				require.Equal(t, "UTC", window.TimeZone, label)
				require.InDelta(t, 5.0, window.Overlay.InputPrice, 1e-9, label)
				require.InDelta(t, 30.0, window.Overlay.OutputPrice, 1e-9, label)
				require.NotNil(t, window.EffectivePricing, label)
				requireSolDisplayTariff(t, ModelDisplayInfo{ModelDisplayTariff: *window.EffectivePricing}, solDisplayStandard, "window tariff "+label)
				if standard {
					require.Equal(t, window.Name, info.ActiveTimeWindow, label)
					// The promotional base stays published, labeled inactive.
					require.NotNil(t, info.InactiveBasePricing, label)
					requireSolDisplayTariff(t, ModelDisplayInfo{ModelDisplayTariff: *info.InactiveBasePricing}, solDisplayPromotional, "inactive base "+label)
				} else {
					require.Empty(t, info.ActiveTimeWindow, label)
					require.Nil(t, info.InactiveBasePricing, label)
				}
				requireDisplayJSONKeys(t, info, standard, label)
			}

			// The administrator tariff keeps precedence and is never rescheduled.
			label := fmt.Sprintf("operator gpt-5.6-sol at %s user=%d", at.Format(time.RFC3339Nano), userID)
			explicit, ok := response.Data[operatorKey].Models["gpt-5.6-sol"]
			require.True(t, ok, label)
			require.InDelta(t, 7.0, explicit.InputPrice, 1e-9, label)
			require.InDelta(t, 21.0, explicit.OutputPrice, 1e-9, label)
			require.InDelta(t, 0.7, explicit.CachedInputPrice, 1e-9, label)
			require.InDelta(t, 8.0, explicit.CacheWrite5mPrice, 1e-9, label)
			require.Empty(t, explicit.Tiers, label)
			require.Empty(t, explicit.TimeWindows, label)
			require.Empty(t, explicit.ActiveTimeWindow, label)
			require.Nil(t, explicit.InactiveBasePricing, label)
			// The unconfigured alias on the same channel follows the native schedule.
			requireSolDisplayTariff(t, response.Data[operatorKey].Models["gpt-5.6"], want, "operator alias "+label)
		}
	}
}

// requireDisplayJSONKeys checks the wire names of the effective-tariff contract.
// Parameters: t is the test handle, info the rendered model, inactive whether a
// window is active (so the base must be published as inactive) and label the
// failure context.
func requireDisplayJSONKeys(t *testing.T, info ModelDisplayInfo, inactive bool, label string) {
	t.Helper()
	encoded, err := json.Marshal(info)
	require.NoError(t, err, label)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields), label)
	for _, key := range []string{"input_price", "cached_input_price", "cache_write_5m_price", "output_price", "tiers", "time_windows"} {
		require.Contains(t, fields, key, label)
	}
	if inactive {
		require.Contains(t, fields, "inactive_base_pricing", label)
		require.Contains(t, fields, "active_time_window", label)
	} else {
		require.NotContains(t, fields, "inactive_base_pricing", label)
	}
	var windows []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["time_windows"], &windows), label)
	require.Len(t, windows, 1, label)
	require.Contains(t, windows[0], "overlay", label)
	require.Contains(t, windows[0], "effective_pricing", label)
}
