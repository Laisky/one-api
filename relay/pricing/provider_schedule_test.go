package pricing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// tieredProviderModel is a provider model with a long-context tier that
// changes every price field, so each inheritance rule is observable.
const tieredProviderModel = "tiered-provider-model"

// tieredProviderConfig returns the provider schedule of tieredProviderModel.
func tieredProviderConfig() adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio: 30, CompletionRatio: 6, CachedInputRatio: 3, CacheWrite5mRatio: 37.5,
		Tiers: []adaptor.ModelRatioTier{{
			InputTokenThreshold: 272_001, Ratio: 60, CompletionRatio: 4.5,
			CachedInputRatio: 6, CacheWrite5mRatio: 75,
		}},
	}
}

// TestResolveModelConfigInheritsProviderTiers verifies a channel entry that
// inherits the provider's base ratio inherits its tiers too, through both
// resolvers, while an explicit operator ratio or tier list keeps its own
// schedule and an inherited tier never undercuts a price the entry states.
func TestResolveModelConfigInheritsProviderTiers(t *testing.T) {
	provider := &MockAdaptor{pricing: map[string]adaptor.ModelConfig{tieredProviderModel: tieredProviderConfig()}}
	at := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		local model.ModelConfigLocal
		// want is the expected tier list; nil means no tiers.
		want []adaptor.ModelRatioTier
	}{
		// Metadata only: the provider schedule applies as a unit, except the
		// cached-input discount the entry never granted (tier input price).
		{name: "metadata_only", local: model.ModelConfigLocal{MaxTokens: 4096},
			want: []adaptor.ModelRatioTier{{InputTokenThreshold: 272_001, Ratio: 60, CompletionRatio: 4.5, CacheWrite5mRatio: 75}}},
		// An explicit operator ratio keeps its own flat schedule.
		{name: "explicit_ratio", local: model.ModelConfigLocal{Ratio: 30, CompletionRatio: 6}},
		// An entry's own tiers replace the provider's.
		{name: "own_tiers", local: model.ModelConfigLocal{Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 1000, Ratio: 40}}},
			want: []adaptor.ModelRatioTier{{InputTokenThreshold: 1000, Ratio: 40}}},
		// Stated prices above the provider tier's are kept.
		{name: "stated_prices_above_tier", local: model.ModelConfigLocal{CompletionRatio: 8, CachedInputRatio: 7, CacheWrite5mRatio: 80},
			want: []adaptor.ModelRatioTier{{InputTokenThreshold: 272_001, Ratio: 60, CompletionRatio: 8, CachedInputRatio: 7, CacheWrite5mRatio: 80}}},
		// Stated prices below the provider tier's are never undercut there.
		{name: "stated_prices_below_tier", local: model.ModelConfigLocal{CompletionRatio: 4, CachedInputRatio: -1, CacheWrite5mRatio: 30},
			want: []adaptor.ModelRatioTier{{InputTokenThreshold: 272_001, Ratio: 60, CompletionRatio: 4.5, CachedInputRatio: 6, CacheWrite5mRatio: 75}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configs := map[string]model.ModelConfigLocal{tieredProviderModel: tc.local}
			full, found := ResolveModelConfig(tieredProviderModel, configs, provider, at)
			require.True(t, found)
			ratioOnly, found := ResolveModelConfigRatioOnly(tieredProviderModel, configs, provider, at)
			require.True(t, found)
			require.Equal(t, tc.want, full.Tiers)
			require.Equal(t, tc.want, ratioOnly.Tiers)
			require.Equal(t, tc.local.Ratio, ratioOnly.Ratio, "the base ratio still falls back through the three layers")
		})
	}
	require.Equal(t, tieredProviderConfig().Tiers, provider.pricing[tieredProviderModel].Tiers, "the provider catalog must not be mutated")
}

// TestResolveModelConfigProviderTierInheritanceBoundaries verifies inheritance
// never borrows a schedule for request-free layer-1 checks (nil provider) or
// operator-tariff-only models, is decided by the entry's base ratio, and lets
// the entry's own windows overlay the inherited schedule.
func TestResolveModelConfigProviderTierInheritanceBoundaries(t *testing.T) {
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{tieredProviderModel: tieredProviderConfig()})
	at := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	metadataOnly := map[string]model.ModelConfigLocal{tieredProviderModel: {MaxTokens: 4096}}

	cfg, found := ResolveModelConfigRatioOnly(tieredProviderModel, metadataOnly, nil, at)
	require.True(t, found)
	require.Empty(t, cfg.Tiers, "a nil provider is a layer-1 check and inherits nothing")

	plain := &MockAdaptor{pricing: map[string]adaptor.ModelConfig{}}
	cfg, found = ResolveModelConfigRatioOnly(tieredProviderModel, metadataOnly, plain, at)
	require.True(t, found)
	require.Len(t, cfg.Tiers, 1, "the global layer's schedule is inherited like the provider's")

	operatorOnly := &operatorTariffMockAdaptor{MockAdaptor: MockAdaptor{pricing: map[string]adaptor.ModelConfig{tieredProviderModel: tieredProviderConfig()}},
		operatorOnly: map[string]bool{tieredProviderModel: true}}
	cfg, found = ResolveModelConfigRatioOnly(tieredProviderModel, metadataOnly, operatorOnly, at)
	require.True(t, found)
	require.Empty(t, cfg.Tiers, "an operator-tariff-only model never borrows a provider schedule")

	windowed := map[string]model.ModelConfigLocal{tieredProviderModel: {TimeWindows: []model.TimeWindowLocal{{
		TimeZone: "UTC", Ranges: []model.ClockRangeLocal{{Start: "11:00", End: "13:00"}},
		Overlay: model.ModelConfigLocal{Ratio: 20, CompletionRatio: 6},
	}}}}
	// The entry omits its base ratio, so it inherits the provider's tiers;
	// its own window then overlays that schedule as a provider window would:
	// the window reprices the base, and the inherited long-context tier stays.
	cfg, _ = ResolveModelConfigRatioOnly(tieredProviderModel, windowed, plain, at)
	require.InDelta(t, 20, cfg.Ratio, 1e-12)
	require.Len(t, cfg.Tiers, 1, "an operator window that only reprices the base keeps the inherited tiers")
	cfg, _ = ResolveModelConfigRatioOnly(tieredProviderModel, windowed, plain, at.Add(6*time.Hour))
	require.Zero(t, cfg.Ratio)
	require.Len(t, cfg.Tiers, 1, "outside the window the inherited schedule applies unchanged")
	ownTiers := map[string]model.ModelConfigLocal{tieredProviderModel: {Ratio: 25, CompletionRatio: 6, TimeWindows: windowed[tieredProviderModel].TimeWindows}}
	cfg, _ = ResolveModelConfigRatioOnly(tieredProviderModel, ownTiers, plain, at)
	require.Empty(t, cfg.Tiers, "an entry with its own base ratio is an explicit operator schedule")
}

// TestValidateExplicitTokenTariffConfigsIgnoresProviderTiers verifies the
// readiness and operator-tariff validators still judge layer 1 alone: a
// provider or global schedule never completes or invalidates a channel tariff.
func TestValidateExplicitTokenTariffConfigsIgnoresProviderTiers(t *testing.T) {
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{tieredProviderModel: tieredProviderConfig()})
	at := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	err := ValidateExplicitTokenTariffConfigs(tieredProviderModel, map[string]model.ModelConfigLocal{tieredProviderModel: {MaxTokens: 4096}}, at)
	require.Error(t, err)
	require.True(t, adaptor.IsRequestPricingNotConfigured(err))
	require.NoError(t, ValidateExplicitTokenTariffConfigs(tieredProviderModel,
		map[string]model.ModelConfigLocal{tieredProviderModel: {Ratio: 30, CompletionRatio: 6}}, at))
}

// windowedProviderModel is a tiered provider model with a midday window whose
// sparse overlay changes every token price, carries its own tiers, and also
// carries an image price that an inherited window must not leak.
const windowedProviderModel = "windowed-provider-model"

// windowedProviderConfig returns the provider schedule of windowedProviderModel.
func windowedProviderConfig() adaptor.ModelConfig {
	cfg := tieredProviderConfig()
	cfg.TimeWindows = []adaptor.TimeWindow{{
		Name: "midday", TimeZone: "UTC", Ranges: []adaptor.ClockRange{{Start: "11:00", End: "13:00"}},
		Overlay: adaptor.ModelConfig{
			Ratio: 40, CompletionRatio: 5, CachedInputRatio: 4, CacheWrite5mRatio: 50,
			Tiers: []adaptor.ModelRatioTier{{InputTokenThreshold: 100_000, Ratio: 80, CompletionRatio: 4, CachedInputRatio: 8}},
			Image: &adaptor.ImagePricingConfig{PricePerImageUsd: 1},
		},
	}}
	return cfg
}

// TestResolveModelConfigInheritsProviderTimeWindows verifies an entry that
// inherits the provider's base ratio and defines no windows inherits the
// provider's windows as part of the schedule: inside the window the window's
// token prices and tiers apply under the same never-undercut and
// no-ungranted-cache-discount rules as tiers, outside it the base tiers do.
func TestResolveModelConfigInheritsProviderTimeWindows(t *testing.T) {
	provider := &MockAdaptor{pricing: map[string]adaptor.ModelConfig{windowedProviderModel: windowedProviderConfig()}}
	midday := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	evening := time.Date(2026, 11, 24, 18, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		local model.ModelConfigLocal
		at    time.Time
		want  adaptor.ModelConfig
	}{
		{name: "metadata_only_in_window", local: model.ModelConfigLocal{MaxTokens: 4096}, at: midday, want: adaptor.ModelConfig{
			Ratio: 40, CompletionRatio: 5, CacheWrite5mRatio: 50, MaxTokens: 4096,
			Tiers: []adaptor.ModelRatioTier{{InputTokenThreshold: 100_000, Ratio: 80, CompletionRatio: 4}},
		}},
		{name: "stated_prices_in_window", local: model.ModelConfigLocal{CompletionRatio: 6, CachedInputRatio: 5, CacheWrite5mRatio: 45}, at: midday, want: adaptor.ModelConfig{
			Ratio: 40, CompletionRatio: 6, CachedInputRatio: 5, CacheWrite5mRatio: 50,
			Tiers: []adaptor.ModelRatioTier{{InputTokenThreshold: 100_000, Ratio: 80, CompletionRatio: 6, CachedInputRatio: 8}},
		}},
		{name: "metadata_only_outside_window", local: model.ModelConfigLocal{MaxTokens: 4096}, at: evening, want: adaptor.ModelConfig{
			MaxTokens: 4096,
			Tiers:     []adaptor.ModelRatioTier{{InputTokenThreshold: 272_001, Ratio: 60, CompletionRatio: 4.5, CacheWrite5mRatio: 75}},
		}},
		{name: "explicit_ratio_in_window", local: model.ModelConfigLocal{Ratio: 30, CompletionRatio: 6}, at: midday, want: adaptor.ModelConfig{
			Ratio: 30, CompletionRatio: 6,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configs := map[string]model.ModelConfigLocal{windowedProviderModel: tc.local}
			ratioOnly, found := ResolveModelConfigRatioOnly(windowedProviderModel, configs, provider, tc.at)
			require.True(t, found)
			full, found := ResolveModelConfig(windowedProviderModel, configs, provider, tc.at)
			require.True(t, found)
			require.Nil(t, full.Image, "an inherited window carries token prices only")
			if tc.at.Equal(evening) {
				// Like a provider config, an unmatched schedule keeps its windows.
				for _, cfg := range []adaptor.ModelConfig{ratioOnly, full} {
					require.Len(t, cfg.TimeWindows, 1)
					require.Zero(t, cfg.TimeWindows[0].Overlay.CachedInputRatio, "no ungranted window cache discount")
					require.Nil(t, cfg.TimeWindows[0].Overlay.Image)
				}
				ratioOnly.TimeWindows, full.TimeWindows = nil, nil
			}
			require.Equal(t, tc.want, ratioOnly)
			require.Equal(t, tc.want, full)
		})
	}

	// An entry with windows of its own keeps them: the provider's window is not
	// inherited, so at midday only the inherited base tiers apply.
	own := map[string]model.ModelConfigLocal{windowedProviderModel: {TimeWindows: []model.TimeWindowLocal{{
		TimeZone: "UTC", Ranges: []model.ClockRangeLocal{{Start: "20:00", End: "21:00"}},
		Overlay: model.ModelConfigLocal{Ratio: 20, CompletionRatio: 6},
	}}}}
	cfg, found := ResolveModelConfigRatioOnly(windowedProviderModel, own, provider, midday)
	require.True(t, found)
	require.Zero(t, cfg.Ratio)
	require.Equal(t, []adaptor.ModelRatioTier{{InputTokenThreshold: 272_001, Ratio: 60, CompletionRatio: 4.5, CacheWrite5mRatio: 75}}, cfg.Tiers)
	require.Equal(t, windowedProviderConfig(), provider.pricing[windowedProviderModel], "the provider catalog must not be mutated")
}
