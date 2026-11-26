package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

// solCutoff is the first instant at which native OpenAI bills GPT-5.6 Sol at
// its standard rate instead of the promotional one.
var solCutoff = time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)

// solInstants returns the request start times every Sol schedule test
// resolves: the last promotional instant, the cutoff and a day after it.
func solInstants() []time.Time {
	return []time.Time{solCutoff.Add(-time.Nanosecond), solCutoff, solCutoff.AddDate(0, 0, 1)}
}

// solUSD is one GPT-5.6 Sol tariff in USD per million tokens below and above
// the 272,000-input-token threshold.
type solUSD struct {
	input, cached, write, output                     float64
	longInput, longCached, longWrite, longOutput     float64
	longThreshold, shortProbeTokens, longProbeTokens int
}

var (
	// solPromotional is the shared OpenAI default every inheriting or
	// fallback consumer keeps on both sides of the cutoff.
	solPromotional = solUSD{4, .40, 5, 20, 8, .80, 10, 30, 272_001, 272_000, 272_001}
	// solStandard is native OpenAI's tariff from the cutoff on.
	solStandard = solUSD{5, .50, 6.25, 30, 10, 1, 12.5, 45, 272_001, 272_000, 272_001}
)

// requireSolTariff checks that cfg bills want at both context sizes.
// Parameters: t is the test handle, cfg the resolved request-time config,
// want the expected tariff and label the failure context.
func requireSolTariff(t *testing.T, cfg adaptor.ModelConfig, want solUSD, label string) {
	t.Helper()
	short := pricing.ResolveEffectivePricingForUsageFromConfig(want.shortProbeTokens, 200, cfg)
	require.Zero(t, short.AppliedTierThreshold, label)
	require.InDelta(t, want.input, short.InputRatio/ratio.MilliTokensUsd, 1e-9, label)
	require.InDelta(t, want.cached, short.CachedInputRatio/ratio.MilliTokensUsd, 1e-9, label)
	require.InDelta(t, want.write, short.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9, label)
	require.InDelta(t, want.output, short.OutputRatio/ratio.MilliTokensUsd, 1e-9, label)

	long := pricing.ResolveEffectivePricingForUsageFromConfig(want.longProbeTokens, 200, cfg)
	require.Equal(t, want.longThreshold, long.AppliedTierThreshold, label)
	require.InDelta(t, want.longInput, long.InputRatio/ratio.MilliTokensUsd, 1e-9, label)
	require.InDelta(t, want.longCached, long.CachedInputRatio/ratio.MilliTokensUsd, 1e-9, label)
	require.InDelta(t, want.longWrite, long.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9, label)
	require.InDelta(t, want.longOutput, long.OutputRatio/ratio.MilliTokensUsd, 1e-9, label)
}

// TestGPT56SolScheduleStaysNativeThroughGlobalFallback drives the production
// global pricing initialization (relay.GetAdaptor) and checks that the native
// OpenAI December schedule never reaches the global fallback catalog. Every
// tariff-eligible channel type whose own catalog lacks the Sol IDs prices them
// through that catalog and must keep the shared default on both sides of the
// cutoff, while native OpenAI switches at the cutoff and explicit channel
// tariffs keep precedence. Parameter t reports any leaked or missing schedule.
func TestGPT56SolScheduleStaysNativeThroughGlobalFallback(t *testing.T) {
	pricing.InitializeGlobalPricingManager(relay.GetAdaptor)
	pricing.ReloadGlobalPricing()

	for _, name := range []string{"gpt-5.6-sol", "gpt-5.6"} {
		global, ok := pricing.GetGlobalModelConfig(name)
		require.True(t, ok, "control: the global catalog must price %s", name)

		native := pricingAdaptorForChannel(channeltype.OpenAI)
		fallbackConsumers := 0
		for _, at := range solInstants() {
			label := fmt.Sprintf("%s at %s", name, at.Format(time.RFC3339Nano))
			cfg, found := pricing.ResolveModelConfig(name, nil, native, at)
			require.True(t, found, label)
			want := solPromotional
			if !at.Before(solCutoff) {
				want = solStandard
			}
			requireSolTariff(t, cfg, want, "native OpenAI "+label)

			for channelType := 1; channelType < channeltype.Dummy; channelType++ {
				provider := pricingAdaptorForChannel(channelType)
				if provider == nil || adaptor.RequiresOperatorTariff(provider, name) {
					continue
				}
				if _, own := provider.GetDefaultModelPricing()[name]; own {
					continue
				}
				fallbackConsumers++
				providerLabel := fmt.Sprintf("%s on %s (%d)", label, provider.GetChannelName(), channelType)
				cfg, found := pricing.ResolveModelConfig(name, nil, provider, at)
				require.True(t, found, providerLabel)
				requireSolTariff(t, cfg, solPromotional, "global fallback "+providerLabel)
				ratioOnly, found := pricing.ResolveModelConfigRatioOnly(name, nil, provider, at)
				require.True(t, found, providerLabel)
				requireSolTariff(t, ratioOnly, solPromotional, "ratio-only global fallback "+providerLabel)
				require.InDelta(t, solPromotional.input*ratio.MilliTokensUsd,
					pricing.ResolveModelRatioAt(name, nil, nil, provider, at), 1e-12, providerLabel)

				// A metadata-only entry inherits the provider schedule as a unit,
				// which for these providers is the global catalog's. It grants no
				// cache-read discount it never set, so cached input bills at the
				// input price (ratio 0), while the inherited tier keeps its write price.
				metadataOnly := map[string]dbmodel.ModelConfigLocal{name: {MaxTokens: 4096}}
				inherited, found := pricing.ResolveModelConfigRatioOnly(name, metadataOnly, provider, at)
				require.True(t, found, providerLabel)
				require.Empty(t, inherited.TimeWindows, providerLabel)
				inherited.Ratio = pricing.ResolveModelRatioAt(name, metadataOnly, nil, provider, at)
				inherited.CompletionRatio = pricing.ResolveCompletionRatioAt(name, metadataOnly, nil, provider, at)
				inheritedWant := solPromotional
				inheritedWant.cached, inheritedWant.write, inheritedWant.longCached = 0, 0, 0
				requireSolTariff(t, inherited, inheritedWant, "inherited schedule "+providerLabel)

				// An administrator tariff keeps precedence over every default layer.
				explicit := map[string]dbmodel.ModelConfigLocal{name: {
					Ratio: 7 * ratio.MilliTokensUsd, CompletionRatio: 3, CachedInputRatio: 0.7 * ratio.MilliTokensUsd,
				}}
				cfg, found = pricing.ResolveModelConfig(name, explicit, provider, at)
				require.True(t, found, providerLabel)
				eff := pricing.ResolveEffectivePricingForUsageFromConfig(272_001, 200, cfg)
				require.Zero(t, eff.AppliedTierThreshold, providerLabel)
				require.InDelta(t, 7.0, eff.InputRatio/ratio.MilliTokensUsd, 1e-9, providerLabel)
				require.InDelta(t, 21.0, eff.OutputRatio/ratio.MilliTokensUsd, 1e-9, providerLabel)
				require.InDelta(t, 0.7, eff.CachedInputRatio/ratio.MilliTokensUsd, 1e-9, providerLabel)
			}
		}
		require.Positive(t, fallbackConsumers, "control: some eligible provider must price %s through the global catalog", name)
		require.Empty(t, global.TimeWindows, "%s: the global fallback catalog must not carry native OpenAI's dated schedule", name)

		// Hosts that inherit OpenAI's shared catalog keep the shared default.
		for _, channelType := range []int{channeltype.Azure, channeltype.AIProxyLibrary, channeltype.Custom} {
			provider := pricingAdaptorForChannel(channelType)
			require.NotNil(t, provider, "channel type %d", channelType)
			for _, at := range solInstants() {
				cfg, found := pricing.ResolveModelConfig(name, nil, provider, at)
				require.True(t, found, "%s on %d", name, channelType)
				requireSolTariff(t, cfg, solPromotional, fmt.Sprintf("%s on inheriting host %d at %s", name, channelType, at))
			}
		}
	}
}
