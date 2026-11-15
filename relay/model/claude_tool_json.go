package model

import (
	"github.com/Laisky/errors/v2"
)

// UnmarshalJSON decodes a caller tool and clears internal provenance even when
// the receiver previously held a trusted server-injected tool.
func (t *ClaudeTool) UnmarshalJSON(data []byte) error {
	type wireTool ClaudeTool
	var decoded wireTool
	if err := DecodeSchemaJSON(data, &decoded); err != nil {
		return errors.Wrap(err, "decode Claude tool")
	}
	*t = ClaudeTool(decoded)
	return nil
}
