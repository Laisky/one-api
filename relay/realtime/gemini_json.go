package realtime

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

// ValidateGeminiJSON rejects duplicate keys, trailing values, and excessively
// nested frames before any routing or billing decode. Parameters: data is a
// size-bounded JSON message. Returns: a payload-free validation error.
func ValidateGeminiJSON(data []byte) error {
	if !utf8.Valid(data) {
		return errors.Wrap(ErrInvalidUsage, "Gemini JSON is not UTF-8")
	}

	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := scanGeminiJSON(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.Wrap(ErrInvalidUsage, "trailing Gemini JSON")
	}
	return nil
}

// scanGeminiJSON validates one value recursively. Parameters: d is the decoder
// and depth bounds nesting. Returns: a validation error without user content.
func scanGeminiJSON(d *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.Wrap(ErrInvalidUsage, "Gemini JSON nesting limit")
	}
	token, err := d.Token()
	if err != nil {
		return errors.Wrap(ErrInvalidUsage, "invalid Gemini JSON")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return errors.Wrap(ErrInvalidUsage, "invalid Gemini object")
			}
			name, ok := key.(string)
			if !ok {
				return errors.Wrap(ErrInvalidUsage, "invalid Gemini key")
			}
			if _, exists := seen[name]; exists {
				return errors.Wrap(ErrInvalidUsage, "duplicate Gemini JSON key")
			}
			seen[name] = struct{}{}
			if err := scanGeminiJSON(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := scanGeminiJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.Wrap(ErrInvalidUsage, "unexpected Gemini delimiter")
	}
	if _, err := d.Token(); err != nil {
		return errors.Wrap(ErrInvalidUsage, "unterminated Gemini JSON")
	}
	return nil
}
