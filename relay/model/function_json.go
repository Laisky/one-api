package model

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// UnmarshalJSON retains exact parameter-schema numbers while preserving the
// existing argument-number and reused-receiver merge behavior of Function.
func (f *Function) UnmarshalJSON(data []byte) error {
	type wireFunction Function
	raw := struct {
		wireFunction
		Parameters json.RawMessage `json:"parameters"`
	}{wireFunction: wireFunction(*f)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "decode function")
	}
	decoded := Function(raw.wireFunction)
	if len(raw.Parameters) > 0 {
		var parameters any
		if err := DecodeSchemaJSON(raw.Parameters, &parameters); err != nil {
			return errors.Wrap(err, "decode function parameters")
		}
		decoded.Parameters = parameters
	}
	*f = decoded
	return nil
}
