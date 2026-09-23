package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPR421LegacyAudioDisplayUnits checks scalar-to-native display conversion
// against exact quota units and proves provider minima and originals survive.
func TestPR421LegacyAudioDisplayUnits(t *testing.T) {
	require.Nil(t, buildLegacyAudioTariffDisplay(nil, 2))
	for _, tc := range []struct {
		name, unit            string
		tokens, quantity, usd float64
	}{
		{"characters", "characters", 0, 1e6, 4},
		{"bytes", "utf8_bytes", 0, 1e6, 4},
		{"seconds_default", "seconds", 0, 3600, 0.144},
		{"seconds_custom", "seconds", 12, 3600, 0.1728},
		{"token_metadata", "", 0, 1000, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := &AudioDisplayPricing{InputUnit: tc.unit, InputPriceQuantity: 1000, InputPriceUsd: 7, PromptTokensPerSecond: tc.tokens, MinimumBillableSeconds: 10, BillingIncrementSeconds: 1}
			snapshot := *base
			got := buildLegacyAudioTariffDisplay(base, 2)
			require.Equal(t, snapshot, *base, "do not change provider catalog metadata")
			require.Equal(t, tc.quantity, got.InputPriceQuantity)
			require.InDelta(t, tc.usd, got.InputPriceUsd, 1e-12)
			require.Equal(t, 10.0, got.MinimumBillableSeconds)
			require.Equal(t, 1.0, got.BillingIncrementSeconds)
		})
	}
}
