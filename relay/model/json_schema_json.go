package model

import "github.com/Laisky/errors/v2"

// UnmarshalJSON decodes exact schema numbers and clears trusted provenance,
// including when caller JSON reconstructs a previously marked receiver.
func (s *JSONSchema) UnmarshalJSON(data []byte) error {
	type wireSchema JSONSchema
	var decoded wireSchema
	if err := DecodeSchemaJSON(data, &decoded); err != nil {
		return errors.Wrap(err, "decode JSON schema")
	}
	*s = JSONSchema(decoded)
	return nil
}
