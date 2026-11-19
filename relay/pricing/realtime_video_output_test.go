package pricing

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// TestResolveRealtimeVideoOutputRatioChannelLayer verifies the channel JSON key
// video.output_token_ratio, its time-window overlay, and that absent, zero or
// corrupt values leave video output unpriced. It accepts t and returns no values.
func TestResolveRealtimeVideoOutputRatioChannelLayer(t *testing.T) {
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{})
	var configs map[string]model.ModelConfigLocal
	require.NoError(t, json.Unmarshal([]byte(`{
		"live": {"ratio": 0.375, "video": {"output_token_ratio": 1.5},
			"time_windows": [{"timezone": "UTC", "ranges": [{"start": "00:00", "end": "06:00"}],
				"overlay": {"video": {"output_token_ratio": 3}}}]},
		"plain": {"ratio": 0.375},
		"zero": {"ratio": 0.375, "video": {"output_token_ratio": 0}}}`), &configs))
	noon := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	ratio, ok := ResolveRealtimeVideoOutputRatio("live", configs, nil, noon)
	require.True(t, ok)
	require.Equal(t, 1.5, ratio)
	ratio, ok = ResolveRealtimeVideoOutputRatio("live", configs, nil, noon.Add(-8*time.Hour))
	require.True(t, ok)
	require.Equal(t, 3.0, ratio, "the active window overlay replaces the ratio")
	for _, name := range []string{"plain", "zero", "unknown"} {
		_, ok = ResolveRealtimeVideoOutputRatio(name, configs, nil, noon)
		require.False(t, ok, name)
	}
	for _, corrupt := range []float64{-1, math.NaN(), math.Inf(1)} {
		stored := map[string]model.ModelConfigLocal{"live": {Ratio: 1, Video: &model.VideoPricingLocal{OutputTokenRatio: corrupt}}}
		_, ok = ResolveRealtimeVideoOutputRatio("live", stored, nil, noon)
		require.False(t, ok, "a corrupt stored ratio %v must never price video output", corrupt)
	}
}

// TestResolveRealtimeVideoOutputRatioProviderLayers verifies the provider and
// global layers, and that an operator-only tariff never borrows them. It
// accepts t and returns no values.
func TestResolveRealtimeVideoOutputRatioProviderLayers(t *testing.T) {
	at := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	withVideo := adaptor.ModelConfig{Ratio: 1, Video: &adaptor.VideoPricingConfig{OutputTokenRatio: 2}}
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{"global-live": {Ratio: 1, Video: &adaptor.VideoPricingConfig{OutputTokenRatio: 4}}})
	provider := &MockAdaptor{pricing: map[string]adaptor.ModelConfig{"provider-live": withVideo}}
	ratio, ok := ResolveRealtimeVideoOutputRatio("provider-live", nil, provider, at)
	require.True(t, ok)
	require.Equal(t, 2.0, ratio)
	ratio, ok = ResolveRealtimeVideoOutputRatio("global-live", nil, provider, at)
	require.True(t, ok)
	require.Equal(t, 4.0, ratio)
	// A channel entry without the field inherits the provider's ratio, like audio.
	ratio, ok = ResolveRealtimeVideoOutputRatio("provider-live", map[string]model.ModelConfigLocal{"provider-live": {Ratio: 1}}, provider, at)
	require.True(t, ok)
	require.Equal(t, 2.0, ratio)

	operatorOnly := &operatorTariffMockAdaptor{MockAdaptor: *provider, operatorOnly: map[string]bool{"provider-live": true, "global-live": true}}
	for _, name := range []string{"provider-live", "global-live"} {
		_, ok = ResolveRealtimeVideoOutputRatio(name, nil, operatorOnly, at)
		require.False(t, ok, "%s: an operator-only tariff never borrows another layer", name)
	}
	ratio, ok = ResolveRealtimeVideoOutputRatio("provider-live",
		map[string]model.ModelConfigLocal{"provider-live": {Ratio: 1, Video: &model.VideoPricingLocal{OutputTokenRatio: 1.25}}}, operatorOnly, at)
	require.True(t, ok)
	require.Equal(t, 1.25, ratio)
}

// TestVideoOutputTokenRatioIsNotGenerationPricing verifies the field survives
// conversion, cloning and window merging, and never turns a video block into a
// video-generation tariff. It accepts t and returns no values.
func TestVideoOutputTokenRatioIsNotGenerationPricing(t *testing.T) {
	cfg := convertLocalVideo(&model.VideoPricingLocal{OutputTokenRatio: 1.5})
	require.Equal(t, 1.5, cfg.OutputTokenRatio)
	require.False(t, cfg.HasData(), "a realtime token ratio is not video-generation pricing")
	require.Nil(t, GetVideoPricingWithThreeLayers("unknown-model", cfg, nil))
	require.Equal(t, 1.5, cfg.Clone().OutputTokenRatio)

	base := &adaptor.VideoPricingConfig{PerSecondUsd: 0.1, OutputTokenRatio: 1.5}
	merged := mergeVideoPricing(base, &adaptor.VideoPricingConfig{PerSecondUsd: 0.2})
	require.Equal(t, 1.5, merged.OutputTokenRatio, "an overlay without the field inherits it")
	require.Equal(t, 0.2, merged.PerSecondUsd)
	merged = mergeVideoPricing(base, &adaptor.VideoPricingConfig{OutputTokenRatio: 3})
	require.Equal(t, 3.0, merged.OutputTokenRatio)
	require.Equal(t, 0.1, merged.PerSecondUsd)
}
