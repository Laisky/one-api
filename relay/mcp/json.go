package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// DecodeJSON decodes exactly one JSON value into destination without rounding opaque numbers.
// The caller must bound data before calling; errors include malformed or trailing JSON.
func DecodeJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return errors.Wrap(err, "decode exact MCP JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return errors.Wrap(err, "decode trailing MCP JSON")
		}
		return errors.New("MCP message must contain exactly one JSON value")
	}
	return nil
}

// IsJSONInteger reports whether a JSON number is mathematically integral without expanding its exponent.
// It runs in linear space/time bounded by the input token, including hostile exponents.
func IsJSONInteger(number json.Number) bool {
	text := string(number)
	if text == "" || (text[0] != '-' && (text[0] < '0' || text[0] > '9')) || !json.Valid([]byte(text)) {
		return false
	}
	mantissa := text
	exponent := int64(0)
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		mantissa = text[:index]
		parsed, err := strconv.ParseInt(text[index+1:], 10, 64)
		if err != nil {
			// A valid JSON exponent can overflow int64. Its sign alone settles
			// integrality for a nonzero, finite-length significand.
			if strings.Trim(mantissa, "-0.") == "" {
				return true
			}
			return !strings.HasPrefix(text[index+1:], "-")
		}
		exponent = parsed
	}
	dot := strings.IndexByte(mantissa, '.')
	fractional := int64(0)
	if dot >= 0 {
		fractional = int64(len(mantissa) - dot - 1)
	}
	if exponent >= fractional {
		return true
	}
	if strings.Trim(mantissa, "-0.") == "" {
		return true
	}
	zeros := int64(0)
	for index := len(mantissa) - 1; index >= 0; index-- {
		if mantissa[index] == '.' {
			continue
		}
		if mantissa[index] != '0' {
			break
		}
		zeros++
	}
	// Rearrangement avoids fractional - exponent overflowing for MinInt64.
	return exponent >= fractional-zeros
}
