package model

import (
	"encoding/json"
	"github.com/Laisky/errors/v2"
)

// UnmarshalJSON decodes known thinking controls and retains unknown siblings.
func (t *Thinking) UnmarshalJSON(raw []byte) error {
	type plain Thinking
	var value plain
	if err := json.Unmarshal(raw, &value); err != nil {
		return errors.Wrap(err, "decode thinking")
	}
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(raw, &extra); err != nil {
		return errors.Wrap(err, "decode thinking fields")
	}
	for _, key := range []string{"type", "budget_tokens", "block_binding"} {
		delete(extra, key)
	}
	if len(extra) == 0 {
		extra = nil
	}
	value.ExtraFields = extra
	*t = Thinking(value)
	return nil
}

// MarshalJSON preserves extension fields while typed controls remain authoritative.
func (t Thinking) MarshalJSON() ([]byte, error) {
	type plain Thinking
	known, err := json.Marshal(plain(t))
	if err != nil {
		return nil, errors.Wrap(err, "encode thinking")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(known, &fields); err != nil {
		return nil, errors.Wrap(err, "decode encoded thinking")
	}
	for key, value := range t.ExtraFields {
		switch key {
		case "type", "budget_tokens", "block_binding":
			continue
		}
		fields[key] = value
	}
	raw, err := json.Marshal(fields)
	return raw, errors.Wrap(err, "encode thinking extensions")
}
