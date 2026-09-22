package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAudioExactTariffRoundTrip preserves rates and denominations through stored
// channel configuration, including explicit free pricing and duration minimums.
func TestAudioExactTariffRoundTrip(t *testing.T) {
	for _, unit := range []string{"characters", "utf8_bytes", "seconds"} {
		for _, price := range []float64{0, .111, 7.15} {
			input := &AudioPricingLocal{InputUnit: unit, InputPriceUsd: price, InputPriceQuantity: 3600, MinimumBillableSeconds: 10, BillingIncrementSeconds: .1}
			channel := &Channel{}
			require.NoError(t, channel.SetModelPriceConfigs(map[string]ModelConfigLocal{"audio": {Audio: input}}))
			got := channel.GetModelPriceConfigs()["audio"].Audio
			require.Equal(t, input, got)
			got.InputPriceUsd = 999
			require.Equal(t, price, channel.GetModelPriceConfigs()["audio"].Audio.InputPriceUsd, "returned values must not alias stored configuration")
		}
	}
}

// TestAudioExactTariffRejectsInvalid validates each monetary and temporal field
// before a malformed rate can enter quota arithmetic or be persisted.
func TestAudioExactTariffRejectsInvalid(t *testing.T) {
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		for _, field := range []string{"price", "quantity", "seconds", "minimum", "increment"} {
			cfg := &AudioPricingLocal{InputUnit: "seconds", InputPriceQuantity: 60, InputPriceUsd: .003}
			switch field {
			case "price":
				cfg.InputPriceUsd = bad
			case "quantity":
				cfg.InputPriceQuantity = bad
			case "seconds":
				cfg.UsdPerSecond = bad
			case "minimum":
				cfg.MinimumBillableSeconds = bad
			case "increment":
				cfg.BillingIncrementSeconds = bad
			}
			_, err := normalizeAudioPricingLocal(cfg)
			require.Error(t, err)
		}
	}
	for _, cfg := range []*AudioPricingLocal{
		{InputUnit: "bananas", InputPriceQuantity: 1}, {InputPriceUsd: 1}, {InputPriceQuantity: 1}, {InputUnit: "characters"},
	} {
		_, err := normalizeAudioPricingLocal(cfg)
		require.Error(t, err)
	}
}
