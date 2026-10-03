package pricing

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestExplicitTokenTariffValidation prevents incomplete, nonfinite and
// numerically invalid rates from reaching an exceptional model's upstream.
func TestExplicitTokenTariffValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		input, output float64
		valid bool
	}{
		{"valid", 0.15, 4, true},
		{"free", 0, 4, false},
		{"missing-output", 1, 0, false},
		{"negative", -1, 4, false},
		{"nan", math.NaN(), 4, false},
		{"infinite-input", math.Inf(1), 4, false},
		{"infinite-output", 1, math.Inf(1), false},
		{"overflow", math.MaxFloat64, 2, false},
		{"underflow", math.SmallestNonzeroFloat64, 0.1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExplicitTokenTariff(adaptor.ModelConfig{Ratio: tc.input, CompletionRatio: tc.output})
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestExplicitTokenTariffActiveWindow verifies admission and settlement use the
// same request-time overlay rather than wall time or the inactive base price.
func TestExplicitTokenTariffActiveWindow(t *testing.T) {
	at := time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC)
	cfg := adaptor.ModelConfig{Ratio: 1, CompletionRatio: 2, TimeWindows: []adaptor.TimeWindow{{
		Name: "active", TimeZone: "UTC", Ranges: []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
		Overlay: adaptor.ModelConfig{Ratio: 3, CompletionRatio: 4},
	}}}
	resolved := ApplyTimeWindowRatioOnly(cfg, at)
	require.NoError(t, validateExplicitTokenTariff(resolved))
	require.Equal(t, float64(3), resolved.Ratio)
	require.Equal(t, float64(4), resolved.CompletionRatio)
	cfg.Tiers = []adaptor.ModelRatioTier{{Ratio: 1, CompletionRatio: 0, OutputTokenThreshold: 1000}}
	require.Error(t, validateExplicitTokenTariff(cfg), "future output tiers must be validated before dispatch")
	cfg.Tiers[0].CompletionRatio = 2
	require.NoError(t, validateExplicitTokenTariff(cfg))
}
