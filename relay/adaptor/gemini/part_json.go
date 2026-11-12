package gemini

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// partJSON is a method-free alias used to encode and decode each native Part once.
type partJSON Part

// MarshalJSON preserves the presence of the native text oneof without adding it to nontext parts.
// Parameters: p is the native part. Returns its JSON representation or a wrapped encoding error.
func (p Part) MarshalJSON() ([]byte, error) {
	if !p.TextPresent && p.Text == "" {
		encoded, err := json.Marshal(partJSON(p))
		if err != nil {
			return nil, errors.Wrap(err, "encode native part")
		}
		return encoded, nil
	}
	encoded, err := json.Marshal(struct {
		*partJSON
		Text *string `json:"text"`
	}{partJSON: (*partJSON)(&p), Text: &p.Text})
	if err != nil {
		return nil, errors.Wrap(err, "encode native text part")
	}
	return encoded, nil
}

// UnmarshalJSON decodes native data once and records whether its text oneof was supplied.
// Parameters: data is one JSON part. Returns a wrapped error for invalid JSON or field types.
func (p *Part) UnmarshalJSON(data []byte) error {
	*p = Part{}
	wire := struct {
		*partJSON
		Text *string `json:"text"`
	}{partJSON: (*partJSON)(p)}
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "decode native part")
	}
	if wire.Text != nil {
		p.Text = *wire.Text
		p.TextPresent = true
	}
	return nil
}
