package adaptor

import "time"

// PricingProvenance records why a catalog tariff is paid or explicitly free.
// Unit identifies the provider's billing unit, not a conversion to token prices.
// ValidUntil is set only when an authoritative expiry is known; a zero value
// does not assert that a promotional price is permanent.
type PricingProvenance struct {
	State      string    `json:"state"`
	Unit       string    `json:"unit"`
	Source     string    `json:"source"`
	VerifiedAt string    `json:"verified_at"`
	ValidUntil time.Time `json:"valid_until,omitempty"`
}
