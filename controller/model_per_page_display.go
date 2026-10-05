package controller

import (
	"github.com/Laisky/one-api/model"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
)

// PerPageDisplayPricing represents flat per-processed-page pricing for display.
// Document models such as layout-parsing OCR services are billed by the number
// of pages they process, never by tokens. Display surfaces both the canonical
// per-1K-pages tariff and the derived per-page USD figure. A present value with
// zero prices is an explicit free tariff, so both fields are always serialized.
type PerPageDisplayPricing struct {
	UsdPerThousandPages float64 `json:"usd_per_thousand_pages"` // USD per 1000 processed pages
	UsdPerPage          float64 `json:"usd_per_page"`           // Derived USD per single processed page
}

// buildPerPageDisplayPricing converts per-page pricing into display data.
// Parameters: cfg is the adaptor-shaped per-page pricing block.
// Returns: display pricing, or nil when the model is not page-priced.
func buildPerPageDisplayPricing(cfg *adaptorpkg.PerPagePricingConfig) *PerPageDisplayPricing {
	if cfg == nil || !cfg.HasData() {
		return nil
	}
	return &PerPageDisplayPricing{
		UsdPerThousandPages: cfg.UsdPerThousandPages,
		UsdPerPage:          cfg.UsdPerThousandPages / 1000.0,
	}
}

// convertLocalPerPageDisplayConfig converts a persisted channel-local per-page
// tariff into its adaptor-shaped form for display rendering.
// Parameters: local is the channel-local per-page tariff, which may be nil.
// Returns: an adaptor-shaped tariff, or nil when local is nil.
func convertLocalPerPageDisplayConfig(local *model.PerPagePricingLocal) *adaptorpkg.PerPagePricingConfig {
	if local == nil {
		return nil
	}
	return &adaptorpkg.PerPagePricingConfig{UsdPerThousandPages: local.UsdPerThousandPages}
}

// applyUnitTariffDisplayOverride reconciles the flat per-call and per-page
// display tariffs after a channel-local override is applied. The two units are
// disjoint billing contracts, and a channel-local config replaces the provider
// default wholesale in the billing resolver, so an inherited per-page tariff is
// shown only when the local override itself declares one; otherwise the model
// bills by its local token or per-call tariff. The inherited per-call display is
// kept unchanged when the override declares no per-page tariff.
// Parameters: converted is the adaptor-shaped local override, perCall and
// perPage are the display tariffs computed so far.
// Returns: the reconciled per-call and per-page display tariffs, and whether
// the inherited provider schedule must be dropped because the local override
// replaced an inherited page tariff that the schedule belonged to.
func applyUnitTariffDisplayOverride(converted adaptorpkg.ModelConfig, perCall *PerCallDisplayPricing, perPage *PerPageDisplayPricing) (*PerCallDisplayPricing, *PerPageDisplayPricing, bool) {
	if converted.PerPage != nil {
		return nil, buildPerPageDisplayPricing(converted.PerPage), perPage != nil
	}
	return perCall, nil, perPage != nil
}
