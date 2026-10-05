package adaptor

import "time"

// Tariff provenance states. A bare zero ratio cannot distinguish these cases, so
// reviewed media models record one explicitly.
const (
	// TariffStateFree marks a provider tariff published as permanently free.
	TariffStateFree = "free"
	// TariffStatePromotionalFree marks a provider tariff that is free for a limited
	// time; ValidUntil is set only when the provider publishes an end date.
	TariffStatePromotionalFree = "promotional_free"
	// TariffStatePaid marks a verified positive provider tariff in Unit.
	TariffStatePaid = "paid"
	// TariffStateUnknown marks a tariff that could not be verified; requests must
	// fail before dispatch until an operator configures an explicit price.
	TariffStateUnknown = "unknown"
	// TariffStateContract marks a privately negotiated tariff that only an
	// operator override can represent.
	TariffStateContract = "contract"
)

// Tariff provenance units name the provider's billing unit.
const (
	// TariffUnitGeneration bills one flat price per generated result (PerCall).
	TariffUnitGeneration = "generation"
	// TariffUnitCharacters bills per input character (Audio.InputUnit "characters").
	TariffUnitCharacters = "characters"
)

// PricingProvenance records why a catalog tariff is paid or explicitly free.
// Unit identifies the provider's billing unit, not a conversion to token prices.
// ValidUntil is set only when an authoritative expiry is known; a zero value
// does not assert that a promotional price is permanent.
type PricingProvenance struct {
	// State is one of the TariffState* constants.
	State string `json:"state"`
	// Unit is one of the TariffUnit* constants.
	Unit string `json:"unit"`
	// Source is the provider page that published the tariff.
	Source string `json:"source"`
	// VerifiedAt is the UTC date (YYYY-MM-DD) the Source was last reviewed.
	VerifiedAt string `json:"verified_at"`
	// ValidUntil is the provider-published UTC expiry, or zero when none is known.
	ValidUntil time.Time `json:"valid_until,omitzero"`
}
