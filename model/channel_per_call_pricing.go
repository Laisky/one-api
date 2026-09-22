package model

import (
	"github.com/Laisky/errors/v2"
	"math"
)

// PerCallPricingLocal stores an explicit USD-per-thousand-invocations tariff.
// A present zero tariff means free; nil means no per-call override.
type PerCallPricingLocal struct {
	UsdPerThousandCalls float64 `json:"usd_per_thousand_calls"`
}

// normalizePerCallPricingLocal validates and copies a persisted per-call tariff.
// It returns nil for an absent tariff and rejects negative or nonfinite amounts.
func normalizePerCallPricingLocal(local *PerCallPricingLocal) (*PerCallPricingLocal, error) {
	if local == nil {
		return nil, nil
	}
	value := local.UsdPerThousandCalls
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, errors.New("per-call USD price must be finite and nonnegative")
	}
	copy := *local
	return &copy, nil
}
