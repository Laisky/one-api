package controller

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// syncResponseReasoning synchronizes typed effort and summary into the wire
// object without discarding provider extensions. Parameters: root is mutated
// and reasoning contains the normalized known fields. Returns: whether known
// fields changed and any JSON error. Unknown numbers never pass through float64.
func syncResponseReasoning(root map[string]json.RawMessage, reasoning *relaymodel.OpenAIResponseReasoning) (bool, error) {
	// A nil object does not authorize deleting opaque provider configuration.
	// Normalizers remove known fields by clearing pointers on a present object.
	if reasoning == nil {
		return false, nil
	}
	object := map[string]json.RawMessage{}
	if raw := root["reasoning"]; len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &object); err != nil {
			return false, errors.Wrap(err, "decode raw reasoning object")
		}
	}
	changed := false
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"effort", reasoning.Effort},
		{"summary", reasoning.Summary},
	} {
		old, present := object[field.name]
		if field.value == nil {
			if present {
				delete(object, field.name)
				changed = true
			}
			continue
		}
		var existing string
		if present && json.Unmarshal(old, &existing) == nil && existing == *field.value {
			continue
		}
		encoded, err := json.Marshal(*field.value)
		if err != nil {
			return false, errors.Wrap(err, "encode reasoning field")
		}
		object[field.name] = encoded
		changed = true
	}
	if !changed {
		return false, nil
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return false, errors.Wrap(err, "encode merged reasoning object")
	}
	root["reasoning"] = encoded
	return true, nil
}
