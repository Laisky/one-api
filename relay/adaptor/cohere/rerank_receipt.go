package cohere

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

var cohereIntegralNumber = regexp.MustCompile(`^([0-9]+)(?:\.([0-9]+))?(?:[eE]([+-]?[0-9]+))?$`)

// parseCohereCount returns the exact nonnegative integer represented by raw and
// whether it fits an int. Bounded lexical parsing avoids float rounding and
// attacker-controlled exponent allocations while accepting 3, 3.0, and 3e0.
func parseCohereCount(raw []byte) (int, bool) {
	if len(raw) > 128 {
		return 0, false
	}
	parts := cohereIntegralNumber.FindStringSubmatch(strings.TrimSpace(string(raw)))
	if parts == nil {
		return 0, false
	}
	exponent := 0
	if parts[3] != "" {
		n, err := strconv.ParseInt(parts[3], 10, 16)
		if err != nil || n < -128 || n > 128 {
			return 0, false
		}
		exponent = int(n)
	}
	digits := strings.TrimLeft(parts[1]+parts[2], "0")
	if digits == "" {
		return 0, true
	}
	scale := exponent - len(parts[2])
	if scale < 0 {
		if -scale > len(digits) || strings.Trim(digits[len(digits)+scale:], "0") != "" {
			return 0, false
		}
		digits = digits[:len(digits)+scale]
	} else {
		if len(digits)+scale > 19 {
			return 0, false
		}
		digits += strings.Repeat("0", scale)
	}
	n, err := strconv.ParseInt(digits, 10, strconv.IntSize)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

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

	count, valid := parseCohereCount(wire.SearchUnits)
	if !valid || count <= 0 {
		units.receiptReason = cohereSearchUnitsInvalid
		return nil
	}
	units.SearchUnits = count
	units.receiptReason = ""
	return nil
}

// UnmarshalJSON decodes integral token counters from raw JSON without rejecting
// equivalent decimal notation, returning a wrapped error for unusable metadata.
func (tokens *RerankTokenUsage) UnmarshalJSON(data []byte) error {
	*tokens = RerankTokenUsage{}
	var wire struct {
		InputTokens  json.RawMessage `json:"input_tokens"`
		OutputTokens json.RawMessage `json:"output_tokens"`
		CachedTokens float64         `json:"cached_tokens,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "decode Cohere token receipt")
	}
	for _, field := range []struct {
		raw    json.RawMessage
		target *int
	}{{wire.InputTokens, &tokens.InputTokens}, {wire.OutputTokens, &tokens.OutputTokens}} {
		if len(field.raw) == 0 || bytes.Equal(bytes.TrimSpace(field.raw), []byte("null")) {
			continue
		}
		count, valid := parseCohereCount(field.raw)
		if !valid {
			return errors.WithStack(errors.New("invalid Cohere token counter"))
		}
		*field.target = count
	}
	tokens.CachedTokens = wire.CachedTokens
	return nil
}

// decodeRerankBillingEnvelope extracts search receipts independently of optional
// token/result fields from a complete JSON body, returning a wrapped parse error.
func decodeRerankBillingEnvelope(body []byte) (*RerankResponse, error) {
	var envelope struct {
		Meta *struct {
			BilledUnits *RerankBilledUnits `json:"billed_units"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, errors.Wrap(err, "decode Cohere billing envelope")
	}
	response := &RerankResponse{}
	if envelope.Meta != nil {
		response.Meta = &RerankMeta{BilledUnits: envelope.Meta.BilledUnits}
	}
	return response, nil
}
