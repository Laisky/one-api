package pricing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// TestMatchTimeWindowScheduleSemantics verifies timezone, midnight, weekday, DST, and local-date matching.
func TestMatchTimeWindowScheduleSemantics(t *testing.T) {
	t.Parallel()

	deepseekWindow := adaptor.TimeWindow{
		TimeZone: "Asia/Shanghai",
		Ranges:   []adaptor.ClockRange{{Start: "00:30", End: "08:30"}},
		Overlay:  adaptor.ModelConfig{Ratio: 0.25},
	}
	require.True(t, MatchTimeWindow(deepseekWindow, time.Date(2026, 6, 29, 19, 0, 0, 0, time.UTC)))
	require.False(t, MatchTimeWindow(deepseekWindow, time.Date(2026, 6, 29, 1, 0, 0, 0, time.UTC)))

	utcWindow := deepseekWindow
	utcWindow.TimeZone = "UTC"
	require.False(t, MatchTimeWindow(utcWindow, time.Date(2026, 6, 29, 16, 45, 0, 0, time.UTC)))
	require.True(t, MatchTimeWindow(deepseekWindow, time.Date(2026, 6, 29, 16, 45, 0, 0, time.UTC)))

	dstWindow := adaptor.TimeWindow{
		TimeZone: "America/New_York",
		Ranges:   []adaptor.ClockRange{{Start: "00:30", End: "08:30"}},
		Overlay:  adaptor.ModelConfig{Ratio: 0.25},
	}
	require.True(t, MatchTimeWindow(dstWindow, time.Date(2026, 3, 8, 6, 0, 0, 0, time.UTC)))
	require.True(t, MatchTimeWindow(dstWindow, time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)))
	require.False(t, MatchTimeWindow(dstWindow, time.Date(2026, 3, 8, 13, 0, 0, 0, time.UTC)))

	mondayNight := adaptor.TimeWindow{
		TimeZone:   "Asia/Shanghai",
		Ranges:     []adaptor.ClockRange{{Start: "22:00", End: "06:00"}},
		DaysOfWeek: []int{int(time.Monday)},
		Overlay:    adaptor.ModelConfig{Ratio: 0.25},
	}
	require.True(t, MatchTimeWindow(mondayNight, time.Date(2026, 6, 29, 15, 30, 0, 0, time.UTC)))
	require.False(t, MatchTimeWindow(mondayNight, time.Date(2026, 6, 29, 21, 30, 0, 0, time.UTC)))

	localDateWindow := adaptor.TimeWindow{
		TimeZone: "Asia/Shanghai",
		Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
		DateFrom: "2026-06-30",
		DateTo:   "2026-07-01",
		Overlay:  adaptor.ModelConfig{Ratio: 0.25},
	}
	localDateInstant := time.Date(2026, 6, 29, 23, 59, 59, 0, time.UTC)
	require.True(t, MatchTimeWindow(localDateWindow, localDateInstant))
	localDateWindow.DateTo = "2026-06-30"
	require.False(t, MatchTimeWindow(localDateWindow, localDateInstant))
}

// TestApplyTimeWindowPrecedenceAndScalarMerge verifies first-match-wins and sparse scalar overlay rules.
func TestApplyTimeWindowPrecedenceAndScalarMerge(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	cfg := adaptor.ModelConfig{
		Ratio:             1,
		CompletionRatio:   4,
		CachedInputRatio:  0.5,
		CacheWrite5mRatio: 0.6,
		CacheWrite1hRatio: 0.7,
		TimeWindows: []adaptor.TimeWindow{
			{
				Name:     "first",
				TimeZone: "UTC",
				Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
				Overlay:  adaptor.ModelConfig{Ratio: 0.25, CachedInputRatio: -1},
			},
			{
				Name:     "second",
				TimeZone: "UTC",
				Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
				Overlay:  adaptor.ModelConfig{Ratio: 0.125, CompletionRatio: 2},
			},
		},
	}

	merged := ApplyTimeWindow(cfg, at)
	require.InDelta(t, 0.25, merged.Ratio, 1e-12)
	require.InDelta(t, 4.0, merged.CompletionRatio, 1e-12)
	require.InDelta(t, -1.0, merged.CachedInputRatio, 1e-12)
	require.InDelta(t, 0.6, merged.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 0.7, merged.CacheWrite1hRatio, 1e-12)
	require.Nil(t, merged.TimeWindows)

	cfg.TimeWindows[0], cfg.TimeWindows[1] = cfg.TimeWindows[1], cfg.TimeWindows[0]
	merged = ApplyTimeWindow(cfg, at)
	require.InDelta(t, 0.125, merged.Ratio, 1e-12)
	require.InDelta(t, 2.0, merged.CompletionRatio, 1e-12)
}

// TestApplyTimeWindowTierMerge verifies inherited and replaced tiers resolve relative to the merged base.
func TestApplyTimeWindowTierMerge(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	base := adaptor.ModelConfig{
		Ratio:           1,
		CompletionRatio: 3,
		Tiers: []adaptor.ModelRatioTier{
			{InputTokenThreshold: 1000, Ratio: 0, CompletionRatio: 2},
		},
		TimeWindows: []adaptor.TimeWindow{{
			TimeZone: "UTC",
			Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
			Overlay:  adaptor.ModelConfig{Ratio: 0.5},
		}},
	}

	merged := ApplyTimeWindow(base, at)
	eff := ResolveEffectivePricingFromConfig(2000, merged)
	require.InDelta(t, 0.5, eff.InputRatio, 1e-12)
	require.InDelta(t, 1.0, eff.OutputRatio, 1e-12)

	base.TimeWindows[0].Overlay.Tiers = []adaptor.ModelRatioTier{
		{InputTokenThreshold: 1000, Ratio: 0, CompletionRatio: 4},
	}
	merged = ApplyTimeWindow(base, at)
	eff = ResolveEffectivePricingFromConfig(2000, merged)
	require.Len(t, merged.Tiers, 1)
	require.InDelta(t, 0.5, eff.InputRatio, 1e-12)
	require.InDelta(t, 2.0, eff.OutputRatio, 1e-12)
}

// TestApplyTimeWindowNestedPricingMerge verifies per-field and map merge rules for nested pricing blocks.
func TestApplyTimeWindowNestedPricingMerge(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	cfg := adaptor.ModelConfig{
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:          0.10,
			BaseResolution:        "720p",
			ResolutionMultipliers: map[string]float64{"720p": 1, "1080p": 2},
		},
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:               10,
			CompletionRatio:           2,
			PromptTokensPerSecond:     8,
			CompletionTokensPerSecond: 16,
			UsdPerSecond:              0.01,
		},
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd:       0.03,
			PromptRatio:            2,
			DefaultSize:            "1024x1024",
			DefaultQuality:         "standard",
			SizeMultipliers:        map[string]float64{"1024x1024": 1},
			QualityMultipliers:     map[string]float64{"standard": 1},
			QualitySizeMultipliers: map[string]map[string]float64{"standard": {"1024x1024": 1}},
		},
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio:  1,
			ImageTokenRatio: 2,
			UsdPerImage:     0.01,
		},
		PerCall: &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 0.2},
		TimeWindows: []adaptor.TimeWindow{{
			TimeZone: "UTC",
			Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
			Overlay: adaptor.ModelConfig{
				Video: &adaptor.VideoPricingConfig{
					BaseResolution:        "1080p",
					ResolutionMultipliers: map[string]float64{"4k": 4},
				},
				Audio: &adaptor.AudioPricingConfig{
					CompletionRatio: 3,
					UsdPerSecond:    0.005,
				},
				Image: &adaptor.ImagePricingConfig{
					PromptRatio:        3,
					DefaultQuality:     "hd",
					SizeMultipliers:    map[string]float64{"2048x2048": 4},
					QualityMultipliers: map[string]float64{"hd": 2},
					QualitySizeMultipliers: map[string]map[string]float64{
						"standard": {"2048x2048": 2},
						"hd":       {"1024x1024": 2},
					},
				},
				Embedding: &adaptor.EmbeddingPricingConfig{
					ImageTokenRatio: 4,
					AudioTokenRatio: 5,
				},
				PerCall: &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 0.1},
			},
		}},
	}

	merged := ApplyTimeWindow(cfg, at)
	require.InDelta(t, 0.10, merged.Video.PerSecondUsd, 1e-12)
	require.Equal(t, "1080p", merged.Video.BaseResolution)
	require.Equal(t, map[string]float64{"720p": 1, "1080p": 2, "4k": 4}, merged.Video.ResolutionMultipliers)
	require.InDelta(t, 10.0, merged.Audio.PromptRatio, 1e-12)
	require.InDelta(t, 3.0, merged.Audio.CompletionRatio, 1e-12)
	require.InDelta(t, 0.005, merged.Audio.UsdPerSecond, 1e-12)
	require.InDelta(t, 0.03, merged.Image.PricePerImageUsd, 1e-12)
	require.Equal(t, "1024x1024", merged.Image.DefaultSize)
	require.Equal(t, "hd", merged.Image.DefaultQuality)
	require.Equal(t, map[string]float64{"1024x1024": 1, "2048x2048": 4}, merged.Image.SizeMultipliers)
	require.Equal(t, map[string]float64{"standard": 1, "hd": 2}, merged.Image.QualityMultipliers)
	require.Equal(t, map[string]map[string]float64{
		"standard": {"1024x1024": 1, "2048x2048": 2},
		"hd":       {"1024x1024": 2},
	}, merged.Image.QualitySizeMultipliers)
	require.InDelta(t, 1.0, merged.Embedding.TextTokenRatio, 1e-12)
	require.InDelta(t, 4.0, merged.Embedding.ImageTokenRatio, 1e-12)
	require.InDelta(t, 5.0, merged.Embedding.AudioTokenRatio, 1e-12)
	require.InDelta(t, 0.1, merged.PerCall.UsdPerThousandCalls, 1e-12)
}

// TestApplyTimeWindowRatioOnlyMergesEmbedding verifies token billing keeps windowed embedding overlays.
func TestApplyTimeWindowRatioOnlyMergesEmbedding(t *testing.T) {
	t.Parallel()

	cfg := adaptor.ModelConfig{
		Ratio: 1,
		Embedding: &adaptor.EmbeddingPricingConfig{
			TextTokenRatio:  1,
			ImageTokenRatio: 2,
		},
		TimeWindows: []adaptor.TimeWindow{{
			TimeZone: "UTC",
			Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
			Overlay: adaptor.ModelConfig{
				Ratio: 0.5,
				Embedding: &adaptor.EmbeddingPricingConfig{
					ImageTokenRatio: 4,
				},
			},
		}},
	}

	merged := ApplyTimeWindowRatioOnly(cfg, time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC))
	require.InDelta(t, 0.5, merged.Ratio, 1e-12)
	require.InDelta(t, 1.0, merged.Embedding.TextTokenRatio, 1e-12)
	require.InDelta(t, 4.0, merged.Embedding.ImageTokenRatio, 1e-12)
	require.Nil(t, merged.Video)
	require.Nil(t, merged.Audio)
	require.Nil(t, merged.Image)
	require.Nil(t, merged.PerCall)
	require.Nil(t, merged.TimeWindows)
}

// TestApplyTimeWindowDoesNotMutateSource verifies overlay merging is safe for shared cached configs.
func TestApplyTimeWindowDoesNotMutateSource(t *testing.T) {
	t.Parallel()

	cfg := adaptor.ModelConfig{
		PerCall: &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 0.2},
		TimeWindows: []adaptor.TimeWindow{{
			TimeZone: "UTC",
			Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
			Overlay: adaptor.ModelConfig{
				PerCall: &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 0.1},
			},
		}},
	}

	merged := ApplyTimeWindow(cfg, time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC))
	require.InDelta(t, 0.1, merged.PerCall.UsdPerThousandCalls, 1e-12)
	require.InDelta(t, 0.2, cfg.PerCall.UsdPerThousandCalls, 1e-12)
	require.NotSame(t, cfg.PerCall, merged.PerCall)
}

// TestResolveModelRatioAtFlatOverridePrecedence verifies legacy flat scalar overrides keep precedence.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestResolveModelRatioAtFlatOverridePrecedence(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	channelConfigs := map[string]model.ModelConfigLocal{
		"windowed-model": {
			Ratio:           1,
			CompletionRatio: 2,
			TimeWindows: []model.TimeWindowLocal{{
				TimeZone: "UTC",
				Ranges:   []model.ClockRangeLocal{{Start: "00:00", End: "00:00"}},
				Overlay:  model.ModelConfigLocal{Ratio: 0.25, CompletionRatio: 4},
			}},
		},
	}

	ratio := ResolveModelRatioAt("windowed-model", channelConfigs, map[string]float64{"windowed-model": 5}, nil, at)
	completionRatio := ResolveCompletionRatioAt("windowed-model", channelConfigs, map[string]float64{"windowed-model": 6}, nil, at)

	require.InDelta(t, 5.0, ratio, 1e-12)
	require.InDelta(t, 6.0, completionRatio, 1e-12)
}

// TestResolveModelRatioAtProviderDefaultWindow verifies adaptor-shipped windows affect scalar helpers.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestResolveModelRatioAtProviderDefaultWindow(t *testing.T) {
	t.Parallel()

	provider := &MockAdaptor{
		name: "windowed-provider",
		pricing: map[string]adaptor.ModelConfig{
			"provider-model": {
				Ratio:           1,
				CompletionRatio: 2,
				TimeWindows: []adaptor.TimeWindow{{
					TimeZone: "UTC",
					Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
					Overlay:  adaptor.ModelConfig{Ratio: 0.25, CompletionRatio: 4},
				}},
			},
		},
	}
	at := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	require.InDelta(t, 0.25, ResolveModelRatioAt("provider-model", nil, nil, provider, at), 1e-12)
	require.InDelta(t, 4.0, ResolveCompletionRatioAt("provider-model", nil, nil, provider, at), 1e-12)
}
