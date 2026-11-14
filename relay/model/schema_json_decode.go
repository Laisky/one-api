package model

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"
)

// DecodeSchemaJSON decodes exactly one schema-bearing JSON value into target
// while retaining numeric text. It must not be used for ordinary argument or
// metadata payloads, whose existing numeric decoding contracts are separate.
func DecodeSchemaJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return errors.Wrap(err, "decode schema JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return errors.Wrap(err, "decode trailing schema JSON")
		}
		return errors.New("schema JSON must contain exactly one value")
	}
	return nil
}
