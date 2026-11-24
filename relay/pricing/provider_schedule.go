package pricing

import (
	"time"

	"github.com/Laisky/one-api/relay/adaptor"
)

// InheritProviderSchedule applies the provider-schedule inheritance rule to a
// channel entry before any time window is applied. An entry that inherits the
// provider's base input price (Ratio 0) inherits the provider's price schedule
// with it: the provider's tiers when the entry defines none, and the
// provider's time windows when the entry defines none. An entry that sets its
// own ratio is an explicit operator schedule and is returned unchanged.
// Inherited windows carry only token prices, adjusted by the same rules as
// inherited tiers (see InheritProviderTiers), so a window never undercuts a
// price the entry states nor grants a cached-input discount it never set.
// Parameters: entry is the channel config with its own windows unapplied;
// provider is the unwindowed provider-default (or global) schedule. Returns:
// the entry, carrying independent copies of the inherited tiers and windows.
func InheritProviderSchedule(entry, provider adaptor.ModelConfig) adaptor.ModelConfig {
	if entry.Ratio != 0 {
		return entry
	}
	if len(entry.TimeWindows) == 0 && len(provider.TimeWindows) > 0 {
		windows := make([]adaptor.TimeWindow, 0, len(provider.TimeWindows))
		for _, window := range provider.TimeWindows {
			window = window.Clone()
			window.Overlay = inheritedWindowOverlay(entry, window.Overlay)
			windows = append(windows, window)
		}
		entry.TimeWindows = windows
	}
	return InheritProviderTiers(entry, provider)
}

// inheritedWindowOverlay returns the token-only overlay an inherited provider
// window applies to entry. Parameters: entry is the channel config and
// overlay the provider window's sparse overlay. Returns: the window's input
// ratio, its other token prices adjusted by inheritedTierPrice, and its tiers
// adjusted by InheritProviderTiers unless the entry defines its own tiers.
func inheritedWindowOverlay(entry, overlay adaptor.ModelConfig) adaptor.ModelConfig {
	inherited := adaptor.ModelConfig{
		Ratio:             overlay.Ratio,
		CompletionRatio:   inheritedTierPrice(entry.CompletionRatio, overlay.CompletionRatio, true),
		CachedInputRatio:  inheritedTierPrice(entry.CachedInputRatio, overlay.CachedInputRatio, false),
		CacheWrite5mRatio: inheritedTierPrice(entry.CacheWrite5mRatio, overlay.CacheWrite5mRatio, true),
		CacheWrite1hRatio: inheritedTierPrice(entry.CacheWrite1hRatio, overlay.CacheWrite1hRatio, true),
	}
	if len(entry.Tiers) == 0 {
		inherited.Tiers = InheritProviderTiers(entry, adaptor.ModelConfig{Tiers: overlay.Tiers}).Tiers
	}
	return inherited
}

// InheritProviderTiers applies the tier half of the provider-schedule rule.
// An entry that inherits the provider's base input price (Ratio 0) and
// defines no tiers of its own inherits the provider's tiers, so a
// metadata-only entry keeps the provider's long-context prices. An entry that
// sets its own ratio or tiers is returned unchanged. Inside an inherited tier
// a price the entry states is never undercut by the provider tier: completion
// ratio and cache prices bill the higher of the two, a completion ratio or
// cache-write price the entry omits follows the provider tier, and a
// cached-input price it omits bills at the tier input price, as the entry does
// below the first threshold. Parameters: entry is the channel config; provider
// supplies the tiers. Returns: the entry, carrying an independent tier copy.
func InheritProviderTiers(entry, provider adaptor.ModelConfig) adaptor.ModelConfig {
	if entry.Ratio != 0 || len(entry.Tiers) > 0 || len(provider.Tiers) == 0 {
		return entry
	}
	tiers := make([]adaptor.ModelRatioTier, 0, len(provider.Tiers))
	for _, tier := range provider.Tiers {
		tier.CompletionRatio = inheritedTierPrice(entry.CompletionRatio, tier.CompletionRatio, true)
		tier.CachedInputRatio = inheritedTierPrice(entry.CachedInputRatio, tier.CachedInputRatio, false)
		tier.CacheWrite5mRatio = inheritedTierPrice(entry.CacheWrite5mRatio, tier.CacheWrite5mRatio, true)
		tier.CacheWrite1hRatio = inheritedTierPrice(entry.CacheWrite1hRatio, tier.CacheWrite1hRatio, true)
		tiers = append(tiers, tier)
	}
	entry.Tiers = tiers
	return entry
}

// inheritedTierPrice returns one price field of an inherited provider tier or
// window. Parameters: entry is the channel entry's own value (0 when omitted,
// negative when free), provider is the provider's value (0 when it keeps the
// earlier value), and keepOmitted reports whether an omitted entry value
// follows the provider (completion ratio, cache writes) instead of billing at
// the input price (cached input, whose provider price is a discount the entry
// never granted). Returns: 0 to keep the entry's or an earlier value,
// otherwise the conservative value.
func inheritedTierPrice(entry, provider float64, keepOmitted bool) float64 {
	switch {
	case provider == 0:
		return 0
	case entry == 0:
		if keepOmitted {
			return provider
		}
		return 0
	default:
		return max(entry, provider)
	}
}

// resolveChannelEntry applies the provider-schedule inheritance rule to a
// channel entry and then its active time window. The entry's base ratio
// decides: an entry that omits it inherits the provider's tiers (and windows,
// when it defines none), and its own windows then overlay that schedule
// exactly as provider windows overlay the provider's, so an operator window
// that only reprices the base keeps the inherited tiers (the higher reading).
// Parameters: cfg is the converted entry with its windows unapplied,
// modelName and provider select the provider schedule, at is the request time
// and apply merges the active window (full or ratio-only). Returns: the
// request-time entry. A nil provider means a request-free layer-1 check,
// which never inherits, and an operator-tariff-only model never borrows a
// provider schedule.
func resolveChannelEntry(cfg adaptor.ModelConfig, modelName string, provider adaptor.Adaptor, at time.Time, apply func(adaptor.ModelConfig, time.Time) adaptor.ModelConfig) adaptor.ModelConfig {
	if cfg.Ratio == 0 && provider != nil && providerTariffEligible(provider, modelName) {
		if schedule, found := providerSchedule(modelName, provider); found {
			cfg = InheritProviderSchedule(cfg, schedule)
		}
	}
	return apply(cfg, at)
}

// providerSchedule returns the unwindowed token price schedule layers 2 and 3
// publish for modelName: the base scalars, tiers and time windows. Parameters:
// modelName is the billed model and provider is non-nil. Returns: the schedule
// and whether either layer publishes tiers or windows for the model.
func providerSchedule(modelName string, provider adaptor.Adaptor) (adaptor.ModelConfig, bool) {
	cfg, found := provider.GetDefaultModelPricing()[modelName]
	if !found {
		cfg, found = GetGlobalModelConfigRatioOnly(modelName)
	}
	if !found || (len(cfg.Tiers) == 0 && len(cfg.TimeWindows) == 0) {
		return adaptor.ModelConfig{}, false
	}
	return adaptor.ModelConfig{Ratio: cfg.Ratio, CompletionRatio: cfg.CompletionRatio, Tiers: cfg.Tiers, TimeWindows: cfg.TimeWindows}, true
}
