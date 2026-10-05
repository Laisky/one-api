package model

import (
	"math"

	"github.com/Laisky/errors/v2"
)

// PerPagePricingLocal stores an explicit USD-per-thousand-processed-pages tariff.
// A present zero tariff means free; nil means the model is not page-priced.
type PerPagePricingLocal struct {
	UsdPerThousandPages float64 `json:"usd_per_thousand_pages"`
}

// normalizePerPagePricingLocal validates and copies a persisted per-page tariff.
// It returns nil for an absent tariff and rejects negative or nonfinite amounts.
func normalizePerPagePricingLocal(local *PerPagePricingLocal) (*PerPagePricingLocal, error) {
	if local == nil {
		return nil, nil
	}
	value := local.UsdPerThousandPages
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, errors.New("per-page USD price must be finite and nonnegative")
	}
	copy := *local
	return &copy, nil
}

// validateUnitTariffsLocal validates the flat per-call and per-page tariffs of
// cfg for modelName. The two units are disjoint billing contracts, so a model
// may declare at most one of them, including across its time-window overlays
// (which merge into the base tariff at request time). It returns a wrapped
// validation error.
func validateUnitTariffsLocal(cfg ModelConfigLocal, modelName string) error {
	if _, err := normalizePerCallPricingLocal(cfg.PerCall); err != nil {
		return errors.Wrapf(err, "validate per-call pricing for %s", modelName)
	}
	if _, err := normalizePerPagePricingLocal(cfg.PerPage); err != nil {
		return errors.Wrapf(err, "validate per-page pricing for %s", modelName)
	}
	perCall, perPage := cfg.PerCall != nil, cfg.PerPage != nil
	for _, window := range cfg.TimeWindows {
		perCall = perCall || window.Overlay.PerCall != nil
		perPage = perPage || window.Overlay.PerPage != nil
	}
	if perCall && perPage {
		return errors.Errorf("model %s cannot declare both per_call and per_page pricing", modelName)
	}
	return nil
}
