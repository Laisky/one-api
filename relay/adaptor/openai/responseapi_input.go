package openai

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// ResponseAPIInput represents the input field that can be either a string or an array
type ResponseAPIInput []any

// UnmarshalJSON implements custom unmarshaling for ResponseAPIInput
// to handle both string and array inputs as per OpenAI Response API specification
func (r *ResponseAPIInput) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*r = ResponseAPIInput{str}
		return nil
	}

	// Validate discovery keys before generic maps can erase duplicate fields.
	discovery, err := InspectResponseToolSearchInput(data)
	if err != nil {
		return errors.Wrap(err, "validate Response API discovery input")
	}

	// If string unmarshaling fails, try as array
	var arr []any
	if err := json.Unmarshal(data, &arr); err != nil {
		return errors.Wrap(err, "ResponseAPIInput.UnmarshalJSON: failed to unmarshal as array")
	}
	// Discovery arguments and schema literals must survive typed rebuilds and
	// state snapshots without a float64 round trip. Ordinary messages retain
	// their existing map representation for content normalization.
	for _, item := range discovery {
		arr[item.Index] = item.Raw
	}
	*r = ResponseAPIInput(arr)
	return nil
}

// MarshalJSON implements custom marshaling for ResponseAPIInput
// If the input contains only one string element, marshal as string
// Otherwise, marshal as array
func (r ResponseAPIInput) MarshalJSON() ([]byte, error) {
	// If there's exactly one element and it's a string, marshal as string
	if len(r) == 1 {
		if str, ok := r[0].(string); ok {
			b, err := json.Marshal(str)
			if err != nil {
				return nil, errors.Wrap(err, "ResponseAPIInput.MarshalJSON: failed to marshal string")
			}
			return b, nil
		}
	}
	// Otherwise, marshal as array
	b, err := json.Marshal([]any(r))
	if err != nil {
		return nil, errors.Wrap(err, "ResponseAPIInput.MarshalJSON: failed to marshal array")
	}
	return b, nil
}
