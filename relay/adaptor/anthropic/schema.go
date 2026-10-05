package anthropic

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// UnmarshalJSON preserves complete JSON Schema constraints alongside typed fields.
func (s *InputSchema) UnmarshalJSON(raw []byte) error {
	type plain InputSchema
	var value plain
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return errors.Wrap(err, "decode Claude tool schema")
	}
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(raw, &extra); err != nil {
		return errors.Wrap(err, "decode Claude schema fields")
	}
	for _, key := range []string{"type", "properties", "required"} {
		delete(extra, key)
	}
	if len(extra) == 0 {
		extra = nil
	}
	value.ExtraFields = extra
	*s = InputSchema(value)
	return nil
}

// MarshalJSON merges schema extensions without allowing them to override typed fields.
func (s InputSchema) MarshalJSON() ([]byte, error) {
	type plain InputSchema
	raw, err := json.Marshal(plain(s))
	if err != nil {
		return nil, errors.Wrap(err, "encode Claude tool schema")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errors.Wrap(err, "decode encoded schema")
	}
	for key, value := range s.ExtraFields {
		switch key {
		case "type", "properties", "required":
			continue
		}
		fields[key] = value
	}
	raw, err = json.Marshal(fields)
	return raw, errors.Wrap(err, "encode complete Claude schema")
}

// claudeInputSchema converts a caller schema without discarding constraints or large integers.
func claudeInputSchema(value any) (InputSchema, error) {
	var schema InputSchema
	if value != nil {
		raw, err := json.Marshal(value)
		if err != nil {
			return schema, errors.Wrap(err, "validation failed: encode tool schema")
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			return schema, errors.Wrap(err, "validation failed: decode tool schema")
		}
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	return schema, nil
}
