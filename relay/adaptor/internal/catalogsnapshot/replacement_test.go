package catalogsnapshot

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestSuppliedSchedulesReplaceBase verifies replacement arrays start with clean
// elements, while absent schedules retain already-normalized prices, using t.
func TestSuppliedSchedulesReplaceBase(t *testing.T) {
	t.Parallel()
	for currency, factor := range map[string]float64{"USD": ratio.MilliTokensUsd, "CNY": ratio.MilliTokensRmb} {
		t.Run(currency, func(t *testing.T) {
			t.Parallel()
			for _, schedule := range []string{"tiers", "time_windows", "both"} {
				t.Run(schedule, func(t *testing.T) {
					t.Parallel()
					original := adaptor.ModelConfig{
						Ratio: 11, CompletionRatio: 2, CachedInputRatio: 3,
						Tiers: []adaptor.ModelRatioTier{{
							Ratio: 2, CompletionRatio: 7, CachedInputRatio: 5,
							CacheWrite5mRatio: 6, CacheWrite1hRatio: 9,
							InputTokenThreshold: 128000, OutputTokenThreshold: 256,
						}},
						TimeWindows: []adaptor.TimeWindow{{
							Name: "old promotion", TimeZone: "Asia/Shanghai",
							DateFrom: "2025-01-01", DateTo: "2025-02-01", DaysOfWeek: []int{1},
							Ranges: []adaptor.ClockRange{{Start: "00:00", End: "06:00"}},
							Overlay: adaptor.ModelConfig{
								Ratio: 3, CompletionRatio: 8, CachedInputRatio: 5,
								CacheWrite5mRatio: 6, CacheWrite1hRatio: 9,
								Tiers: []adaptor.ModelRatioTier{{Ratio: 7}},
								Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.2},
							},
						}},
					}
					base := map[string]adaptor.ModelConfig{"model": original.Clone()}
					tierPatch := `"tiers":[{"ratio":4,"completion_ratio":3,"input_token_threshold":4096}]`
					windowPatch := `"time_windows":[{"ranges":[{"start":"01:00","end":"02:00"}],"overlay":{"ratio":6,"completion_ratio":2}}]`
					patch := tierPatch
					if schedule == "time_windows" {
						patch = windowPatch
					} else if schedule == "both" {
						patch += "," + windowPatch
					}
					raw := []byte(fmt.Sprintf(`{"version":1,"currency":%q,"sources":[{"url":"fixture"}],"models":{"model":{%s}}}`, currency, patch))
					got, err := Decode(base, raw)
					require.NoError(t, err)
					want := original.Clone()
					if schedule != "time_windows" {
						want.Tiers = []adaptor.ModelRatioTier{{Ratio: 4 * factor, CompletionRatio: 3, InputTokenThreshold: 4096}}
					}
					if schedule != "tiers" {
						want.TimeWindows = []adaptor.TimeWindow{{
							Ranges:  []adaptor.ClockRange{{Start: "01:00", End: "02:00"}},
							Overlay: adaptor.ModelConfig{Ratio: 6 * factor, CompletionRatio: 2},
						}}
					}
					require.Equal(t, want, got["model"], "omitted fields in replacement elements must not inherit old prices or dates")
					got["model"].Tiers[0].CachedInputRatio = 123
					got["model"].TimeWindows[0].Ranges[0].Start = "12:00"
					require.Equal(t, original, base["model"], "decoding and caller mutation must leave base independent")
				})
			}
		})
	}
}

// TestEmptySchedulesClearAndInvalidSchedulesAreAtomic checks empty/null arrays
// clear both schedules and invalid replacement data leaves base unchanged with t.
func TestEmptySchedulesClearAndInvalidSchedulesAreAtomic(t *testing.T) {
	t.Parallel()
	original := adaptor.ModelConfig{Ratio: 11, CompletionRatio: 2,
		Tiers:       []adaptor.ModelRatioTier{{Ratio: 2, CachedInputRatio: 5}},
		TimeWindows: []adaptor.TimeWindow{{Name: "old", Overlay: adaptor.ModelConfig{Ratio: 3}}},
	}
	for _, value := range []string{"[]", "null"} {
		base := map[string]adaptor.ModelConfig{"model": original.Clone()}
		raw := []byte(fmt.Sprintf(`{"version":1,"currency":"USD","sources":[{"url":"fixture"}],"models":{"model":{"tiers":%s,"time_windows":%s}}}`, value, value))
		got, err := Decode(base, raw)
		require.NoError(t, err)
		require.Empty(t, got["model"].Tiers)
		require.Empty(t, got["model"].TimeWindows)
		require.Equal(t, original, base["model"])
	}
	base := map[string]adaptor.ModelConfig{"model": original.Clone()}
	got, err := Decode(base, []byte(`{"version":1,"currency":"USD","sources":[{"url":"fixture"}],"models":{"model":{"tiers":[{"ratio":4}],"time_windows":[{"unknown":1}]}}}`))
	require.Error(t, err)
	require.Nil(t, got)
	require.Equal(t, original, base["model"])
}

// TestSnapshotCatalogUnion checks growing the result map preserves overlapping
// and disjoint models, including a nil base, without changing input maps with t.
func TestSnapshotCatalogUnion(t *testing.T) {
	t.Parallel()
	for _, base := range []map[string]adaptor.ModelConfig{nil, {"kept": {Ratio: 7}, "updated": {Ratio: 9}}} {
		got, err := Decode(base, []byte(`{"version":1,"currency":"USD","sources":[{"url":"fixture"}],"models":{"updated":{"ratio":2,"completion_ratio":3},"new":{"ratio":4,"completion_ratio":2}}}`))
		require.NoError(t, err)
		require.Equal(t, 2*ratio.MilliTokensUsd, got["updated"].Ratio)
		require.Equal(t, 4*ratio.MilliTokensUsd, got["new"].Ratio)
		if base == nil {
			require.Len(t, got, 2)
		} else {
			require.Len(t, got, 3)
			require.Equal(t, base["kept"], got["kept"])
			require.Equal(t, 9.0, base["updated"].Ratio)
			require.Len(t, base, 2)
		}
	}
}
