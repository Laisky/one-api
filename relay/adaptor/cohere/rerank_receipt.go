package cohere

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

const (
	cohereSearchUnitsMissing = "cohere_search_units_missing"
	cohereSearchUnitsInvalid = "cohere_search_units_invalid"
)

// UnmarshalJSON parses an integral search-unit receipt without allowing malformed
// billing metadata to discard an otherwise useful successful provider response.
// Missing and invalid counts remain distinct, and reused receivers are reset.
func (units *RerankBilledUnits) UnmarshalJSON(data []byte) error {
	*units = RerankBilledUnits{receiptReason: cohereSearchUnitsMissing}
	var wire struct {
		SearchUnits json.RawMessage `json:"search_units"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "decode Cohere billed-unit object")
	}
	if len(wire.SearchUnits) == 0 || bytes.Equal(bytes.TrimSpace(wire.SearchUnits), []byte("null")) {
		return nil
	}

	var count int64
	if err := json.Unmarshal(wire.SearchUnits, &count); err != nil {
		units.receiptReason = cohereSearchUnitsInvalid
		return nil
	}
	if count <= 0 || int64(int(count)) != count {
		units.receiptReason = cohereSearchUnitsInvalid
		return nil
	}
	units.SearchUnits = int(count)
	units.receiptReason = ""
	return nil
}
