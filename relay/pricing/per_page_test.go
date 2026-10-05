package pricing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// TestResolveModelConfigCarriesPerPagePricing verifies a channel per_page
// tariff, including an explicit free time-window overlay, survives resolution,
// and that token-only resolution never mistakes it for token pricing data.
func TestResolveModelConfigCarriesPerPagePricing(t *testing.T) {
	t.Parallel()
	configs := map[string]model.ModelConfigLocal{"glm-ocr": {
		PerPage: &model.PerPagePricingLocal{UsdPerThousandPages: 4},
		TimeWindows: []model.TimeWindowLocal{{
			TimeZone: "UTC",
			Ranges:   []model.ClockRangeLocal{{Start: "00:00", End: "06:00"}},
			Overlay:  model.ModelConfigLocal{PerPage: &model.PerPagePricingLocal{UsdPerThousandPages: 0}},
		}},
	}}

	day, found := ResolveModelConfig("glm-ocr", configs, nil, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))
	require.True(t, found)
	require.NotNil(t, day.PerPage)
	require.InDelta(t, 4, day.PerPage.UsdPerThousandPages, 1e-12)
	require.Nil(t, day.PerCall)

	night, found := ResolveModelConfig("glm-ocr", configs, nil, time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC))
	require.True(t, found)
	require.NotNil(t, night.PerPage, "a free overlay is an explicit tariff, not an absent one")
	require.Zero(t, night.PerPage.UsdPerThousandPages)

	ratioOnly, found := ResolveModelConfigRatioOnly("glm-ocr", configs, nil, time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC))
	require.True(t, found)
	require.Nil(t, ratioOnly.PerPage)
}

// TestPerPagePricingCloneIsolation verifies cloned configurations never share
// the per-page tariff pointer with cached catalog entries.
func TestPerPagePricingCloneIsolation(t *testing.T) {
	t.Parallel()
	base := adaptor.ModelConfig{PerPage: &adaptor.PerPagePricingConfig{UsdPerThousandPages: 2}}
	clone := base.Clone()
	clone.PerPage.UsdPerThousandPages = 9
	require.InDelta(t, 2, base.PerPage.UsdPerThousandPages, 1e-12)
	require.True(t, base.PerPage.HasData())
	require.False(t, (*adaptor.PerPagePricingConfig)(nil).HasData())
}
